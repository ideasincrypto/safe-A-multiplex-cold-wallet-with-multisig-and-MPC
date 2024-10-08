package observer

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/MixinNetwork/mixin/logger"
	"github.com/MixinNetwork/safe/apps/ethereum"
	"github.com/MixinNetwork/safe/common"
	"github.com/MixinNetwork/safe/common/abi"
	"github.com/MixinNetwork/safe/keeper/store"
	"github.com/MixinNetwork/trusted-group/mtg"
	gc "github.com/ethereum/go-ethereum/common"
	"github.com/fox-one/mixin-sdk-go/v2"
	"github.com/gofrs/uuid/v5"
	"github.com/shopspring/decimal"
)

func (node *Node) listOutputs(ctx context.Context, asset string, state mixin.SafeUtxoState) ([]*mixin.SafeUtxo, error) {
	for {
		outputs, err := node.mixin.SafeListUtxos(ctx, mixin.SafeListUtxoOption{
			Members:   []string{node.mixin.ClientID},
			Threshold: 1,
			Asset:     asset,
			State:     state,
		})
		if err != nil {
			reason := strings.ToLower(err.Error())
			switch {
			case strings.Contains(reason, "timeout"):
			case strings.Contains(reason, "eof"):
			case strings.Contains(reason, "handshake"):
			default:
				return nil, err
			}
			time.Sleep(2 * time.Second)
			continue
		}
		return outputs, nil
	}
}

func (node *Node) fetchPolygonBondAsset(ctx context.Context, entry string, chain byte, assetId, assetAddress, holder string) (*Asset, *Asset, string, error) {
	asset, err := node.fetchAssetMetaFromMessengerOrEthereum(ctx, assetId, assetAddress, chain)
	if err != nil {
		return nil, nil, "", fmt.Errorf("node.fetchAssetMeta(%s) => %v", assetId, err)
	}

	addr := abi.GetFactoryAssetAddress(entry, assetId, asset.Symbol, asset.Name, holder)
	assetKey := strings.ToLower(addr.String())
	err = ethereum.VerifyAssetKey(assetKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("mvm.VerifyAssetKey(%s) => %v", assetKey, err)
	}

	bondId := ethereum.GenerateAssetId(common.SafeChainPolygon, assetKey)
	bond, err := node.fetchAssetMeta(ctx, bondId)
	return asset, bond, bondId, err
}

func (node *Node) checkOrDeployPolygonBond(ctx context.Context, entry string, chain byte, assetId, assetAddress, holder string) (bool, error) {
	asset, bond, _, err := node.fetchPolygonBondAsset(ctx, entry, chain, assetId, assetAddress, holder)
	if err != nil {
		return false, fmt.Errorf("node.fetchPolygonBondAsset(%s, %s) => %v", assetId, holder, err)
	}
	if bond != nil {
		return true, nil
	}
	rpc, key := node.conf.PolygonRPC, node.conf.EVMKey
	return false, abi.GetOrDeployFactoryAsset(ctx, rpc, key, assetId, asset.Symbol, asset.Name, entry, holder)
}

func (node *Node) deployPolygonBondAssets(ctx context.Context, safes []*store.Safe, receiver string) error {
	for _, safe := range safes {
		switch safe.Chain {
		case common.SafeChainBitcoin, common.SafeChainLitecoin:
			_, assetId := node.bitcoinParams(safe.Chain)
			_, err := node.checkOrDeployPolygonBond(ctx, receiver, safe.Chain, assetId, "", safe.Holder)
			if err != nil {
				return err
			}
		case common.SafeChainEthereum, common.SafeChainPolygon:
			_, assetId := node.ethereumParams(safe.Chain)
			balances, err := node.keeperStore.ReadAllEthereumTokenBalances(ctx, safe.Address)
			if err != nil {
				return err
			}
			_, err = node.checkOrDeployPolygonBond(ctx, receiver, safe.Chain, assetId, "", safe.Holder)
			if err != nil {
				return err
			}
			for _, balance := range balances {
				_, err = node.checkOrDeployPolygonBond(ctx, receiver, safe.Chain, balance.AssetId, balance.AssetAddress, safe.Holder)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (node *Node) distributePolygonBondAsset(ctx context.Context, receiver string, safe *store.Safe, bond *Asset, amount decimal.Decimal) error {
	inputs, err := node.listOutputs(ctx, bond.AssetId, mixin.SafeUtxoStateUnspent)
	if err != nil || len(inputs) == 0 {
		return err
	}
	total := decimal.NewFromInt(0)
	for _, o := range inputs {
		total = total.Add(o.Amount)
	}

	traceId := common.UniqueId(bond.AssetId, safe.RequestId)
	crv := byte(common.CurveSecp256k1ECDSABitcoin)
	extra := gc.HexToAddress(receiver).Bytes()
	switch safe.Chain {
	case common.SafeChainBitcoin:
	case common.SafeChainLitecoin:
		crv = common.CurveSecp256k1ECDSALitecoin
	case common.SafeChainEthereum:
		crv = common.CurveSecp256k1ECDSAEthereum
	case common.SafeChainPolygon:
		crv = common.CurveSecp256k1ECDSAPolygon
	default:
		panic(safe.Chain)
	}
	op := &common.Operation{
		Id:     traceId,
		Type:   common.ActionMigrateSafeToken,
		Curve:  crv,
		Public: safe.Holder,
		Extra:  extra,
	}
	members := node.keeper.Genesis.Members
	threshold := node.keeper.Genesis.Threshold
	traceId = fmt.Sprintf("OBSERVER:%s:KEEPER:%v:%d", node.conf.App.AppId, members, threshold)
	traceId = node.safeTraceId(traceId, op.Id)

	memo := mtg.EncodeMixinExtraBase64(node.conf.KeeperAppId, op.Encode())
	if len(extra) > 160 {
		panic(fmt.Errorf("node.sendKeeperTransaction(%v) omitted %x", op, extra))
	}

	b := mixin.NewSafeTransactionBuilder(inputs)
	b.Memo = memo
	b.Hint = traceId

	keeperShare := total.Sub(amount)
	if !keeperShare.IsPositive() {
		panic(keeperShare)
	}
	outputs := []*mixin.TransactionOutput{
		{
			Address: mixin.RequireNewMixAddress(node.keeper.Genesis.Members, byte(node.keeper.Genesis.Threshold)),
			Amount:  keeperShare,
		},
	}
	if amount.IsPositive() {
		outputs = append(outputs, &mixin.TransactionOutput{
			Address: mixin.RequireNewMixAddress(safe.Receivers, byte(safe.Threshold)),
			Amount:  amount,
		})
	}

	tx, err := node.mixin.MakeTransaction(ctx, b, outputs)
	if err != nil {
		return err
	}
	raw, err := tx.Dump()
	if err != nil {
		return err
	}
	req, err := common.CreateTransactionRequestUntilSufficient(ctx, node.mixin, traceId, raw)
	if err != nil {
		return err
	}
	_, err = common.SignTransactionUntilSufficient(ctx, node.mixin, req.RequestID, req.RawTransaction, req.Views, node.conf.App.SpendPrivateKey)
	return err
}

func userNotRegistered(err error) bool {
	return strings.Contains(err.Error(), "User is not registered")
}

func (node *Node) distributePolygonBondAssetsForSafe(ctx context.Context, safe *store.Safe, receiver string) (bool, bool, error) {
	switch safe.Chain {
	case common.SafeChainBitcoin, common.SafeChainLitecoin:
		_, assetId := node.bitcoinParams(safe.Chain)
		_, bond, _, err := node.fetchPolygonBondAsset(ctx, receiver, safe.Chain, assetId, "", safe.Holder)
		if err != nil || bond == nil {
			return false, false, err
		}
		outputs, err := node.keeperStore.ListAllBitcoinUTXOsForHolder(ctx, safe.Holder)
		if err != nil {
			return false, false, err
		}
		var total int64
		for _, o := range outputs {
			total += o.Satoshi
		}
		err = node.distributePolygonBondAsset(ctx, receiver, safe, bond, decimal.NewFromInt(total).Div(decimal.New(1, 8)))
		logger.Printf("MigrateSafeAssets() => distributePolygonBondAsset(%v, %s) => %v", safe, receiver, err)
		if err != nil {
			if userNotRegistered(err) {
				return false, true, nil
			}
			return false, false, err
		}
		return true, false, nil
	case common.SafeChainEthereum, common.SafeChainPolygon:
		balances, err := node.keeperStore.ReadAllEthereumTokenBalances(ctx, safe.Address)
		if err != nil {
			return false, false, err
		}
		pendings, err := node.keeperStore.ReadUnfinishedTransactionsByHolder(ctx, safe.Address)
		if err != nil {
			return false, false, err
		}
		bs, _ := viewBalances(balances, pendings)

		_, chainAssetId := node.ethereumParams(safe.Chain)
		if _, ok := bs[chainAssetId]; !ok {
			bs[chainAssetId] = &AssetBalance{
				AssetAddress: ethereum.EthereumEmptyAddress,
				Amount:       "0",
			}
		}

		for assetId, balance := range bs {
			asset, bond, _, err := node.fetchPolygonBondAsset(ctx, receiver, safe.Chain, assetId, balance.AssetAddress, safe.Holder)
			if err != nil || bond == nil {
				return false, false, err
			}
			cur, _ := new(big.Int).SetString(balance.Amount, 10)
			amt := decimal.NewFromBigInt(cur, -int32(asset.Decimals))
			err = node.distributePolygonBondAsset(ctx, receiver, safe, bond, amt)
			logger.Printf("MigrateSafeAssets() => distributePolygonBondAsset(%v, %s) => %v", safe, receiver, err)
			if err != nil {
				if userNotRegistered(err) {
					return false, true, nil
				}
				return false, false, err
			}
		}
		return true, false, nil
	default:
		panic(safe.Chain)
	}
}

func (node *Node) distributePolygonBondAssets(ctx context.Context, safes []*store.Safe, receiver string) error {
	for {
		allHandled := true

		for _, safe := range safes {
			handled, skip, err := node.distributePolygonBondAssetsForSafe(ctx, safe, receiver)
			logger.Printf("MigrateSafeAssets() => distributePolygonBondAssetsForSafe(%v, %s) => %t %t %v", safe, receiver, handled, skip, err)
			if err != nil {
				return err
			}
			if skip {
				continue
			}
			if handled {
				err = node.store.MarkAccountMigrated(ctx, safe.Address)
				if err != nil {
					return err
				}
			} else {
				allHandled = false
			}
		}

		if allHandled {
			return nil
		}

		time.Sleep(1 * time.Minute)
	}
}

// FIXME remove this
func (s *SQLite3Store) MigrateDB(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	key, val := "SCHEMA:VERSION:cb8eb5d5b6d530dfa79bd35c2c74deaf96c3485b", ""
	row := tx.QueryRowContext(ctx, "SELECT value FROM properties WHERE key=?", key)
	err = row.Scan(&val)
	if err == nil {
		return nil
	} else if err != sql.ErrNoRows {
		return err
	}

	rows, err := tx.QueryContext(ctx, "SELECT transaction_hash,output_index FROM deposits WHERE request_id IS NULL AND state=3")
	if err != nil {
		return err
	}
	var count int
	for rows.Next() {
		var hash string
		var index int64
		err := rows.Scan(&hash, &index)
		if err != nil {
			return err
		}
		id := uuid.Must(uuid.NewV4()).String()
		sql := "UPDATE deposits SET request_id=? WHERE transaction_hash=? AND output_index=? AND request_id IS NULL AND state=3"
		err = s.execOne(ctx, tx, sql, id, hash, index)
		if err != nil {
			return err
		}
		count = count + 1
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, "INSERT INTO properties (key, value, created_at, updated_at) VALUES (?, ?, ?, ?)", key, fmt.Sprint(count), now, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (node *Node) MigrateSafeAssets(ctx context.Context) error {
	entry := node.conf.PolygonObserverDepositEntry
	safes, err := node.keeperStore.ListSafesWithState(ctx, common.RequestStateDone)
	if err != nil {
		return fmt.Errorf("store.ListSafesWithState() => %v", err)
	}
	unmigrated := []*store.Safe{}
	for _, safe := range safes {
		err := node.store.MarkAccountApproved(ctx, safe.Address)
		if err != nil {
			return fmt.Errorf("store.MarkAccountApproved(%s) => %v", safe.Address, err)
		}
		acc, err := node.store.ReadAccount(ctx, safe.Address)
		if err != nil {
			return err
		}
		if !acc.MigratedAt.Valid {
			unmigrated = append(unmigrated, safe)
		}
	}

	logger.Printf("MigrateSafeAssets(%d) => %d", len(safes), len(unmigrated))
	err = node.deployPolygonBondAssets(ctx, unmigrated, entry)
	if err != nil {
		return fmt.Errorf("node.deployPolygonBondAssets(%s) => %v", entry, err)
	}

	err = node.distributePolygonBondAssets(ctx, unmigrated, entry)
	if err != nil {
		return fmt.Errorf("node.distributePolygonBondAssets(%s) => %v", entry, err)
	}
	return nil
}
