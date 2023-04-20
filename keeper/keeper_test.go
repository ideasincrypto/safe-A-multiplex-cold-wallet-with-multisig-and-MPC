package keeper

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MixinNetwork/mixin/crypto"
	"github.com/MixinNetwork/mixin/domains/mvm"
	"github.com/MixinNetwork/mixin/logger"
	"github.com/MixinNetwork/nfo/store"
	"github.com/MixinNetwork/safe/apps/bitcoin"
	"github.com/MixinNetwork/safe/common"
	"github.com/MixinNetwork/safe/common/abi"
	"github.com/MixinNetwork/safe/signer"
	"github.com/MixinNetwork/trusted-group/mtg"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/fox-one/mixin-sdk-go"
	"github.com/gofrs/uuid"
	"github.com/pelletier/go-toml"
	"github.com/shopspring/decimal"
	"github.com/test-go/testify/assert"
	"github.com/test-go/testify/require"
)

const (
	testAccountPriceAssetId          = "31d2ea9c-95eb-3355-b65b-ba096853bc18"
	testBondAssetId                  = "9390ca53-2ec5-3a65-acee-9e149ae38452"
	testSafeBondReceiverId           = "e459de8b-4edd-44ff-a119-b1d707f8521a"
	testBitcoinKeyHolderPrivate      = "04b1d2c7d2e9c630d840fc9ba452617d6d963ceba43b31d7f16403612d08353c"
	testBitcoinKeyObserverPrivate    = "d2b9dabc8745d0f15956dc6808c33910bb455bb01ed04cdf4e56f88da76d48c1"
	testBitcoinKeyAccountantPrivate  = "743602ca7e2723e7dd510e611957cfdcbb517dff0c5b1877472e35b0f19e0063"
	testBitcoinKeyDummyHolderPrivate = "75d5f311c8647e3a1d84a0d975b6e50b8c6d3d7f195365320077f41c6a165155"
	testSafePublicKey                = "bc1q7erq8pvv665nuzmrqrn5vyc3kcd8v4vtafvdd8mkt9h05qz57l3qks2lsd"
	testSafeAccountant               = "bc1qkczcrtknyhs228xg9nvujjvs96x56cf9l4q9za"
	testTransactionReceiver          = "bc1ql0up0wwazxt6xlj84u9fnvhnagjjetcn7h4z5xxvd0kf5xuczjgqq2aehc"
	testBitcoinDepositMainHash       = "8260f125afdb1a85b540f0066cd9db18d488a3891b5fa5595c73f40435502d09"
)

func TestKeeper(t *testing.T) {
	logger.SetLevel(logger.VERBOSE)
	require := require.New(t)
	ctx, signers := signer.TestPrepare(assert.New(t))
	mpc := signer.TestCMPPrepareKeys(ctx, assert.New(t), signers, common.CurveSecp256k1ECDSABitcoin)

	root, err := os.MkdirTemp("", "safe-keeper-test")
	require.Nil(err)
	node := testBuildNode(ctx, require, root)
	require.NotNil(node)
	timestamp, err := node.timestamp(ctx)
	require.Nil(err)
	require.Equal(time.Unix(0, node.conf.MTG.Genesis.Timestamp), timestamp)
	testSpareKeys(ctx, require, node, 0, 0, 0, 0)

	id := uuid.Must(uuid.NewV4()).String()
	out := testBuildSignerOutput(node, id, mpc, common.OperationTypeKeygenOutput, []byte{common.RequestRoleSigner})
	testStep(ctx, require, node, out)
	v, err := node.store.ReadProperty(ctx, id)
	require.Nil(err)
	require.Equal("", v)
	testSpareKeys(ctx, require, node, 0, 1, 0, 0)

	id = uuid.Must(uuid.NewV4()).String()
	observer := testPublicKey(testBitcoinKeyObserverPrivate)
	out = testBuildObserverRequest(node, id, observer, common.ActionObserverAddKey, []byte{common.RequestRoleObserver})
	testStep(ctx, require, node, out)
	v, err = node.store.ReadProperty(ctx, id)
	require.Nil(err)
	require.Equal("", v)
	testSpareKeys(ctx, require, node, 0, 1, 1, 0)

	id = uuid.Must(uuid.NewV4()).String()
	accountant := testPublicKey(testBitcoinKeyAccountantPrivate)
	out = testBuildObserverRequest(node, id, accountant, common.ActionObserverAddKey, []byte{common.RequestRoleAccountant})
	testStep(ctx, require, node, out)
	v, err = node.store.ReadProperty(ctx, id)
	require.Nil(err)
	require.Equal("", v)
	wka, err := bitcoin.BuildWitnessKeyAccount(accountant)
	require.Nil(err)
	require.Equal(testSafeAccountant, wka.Address)
	testSpareKeys(ctx, require, node, 0, 1, 1, 1)

	batch := byte(64)
	id = uuid.Must(uuid.NewV4()).String()
	dummy := testPublicKey(testBitcoinKeyDummyHolderPrivate)
	out = testBuildObserverRequest(node, id, dummy, common.ActionObserverRequestSignerKeys, []byte{batch})
	testStep(ctx, require, node, out)
	for i := byte(0); i < batch; i++ {
		pid := mixin.UniqueConversationID(id, fmt.Sprintf("%8d", i))
		pid = mixin.UniqueConversationID(pid, fmt.Sprintf("MTG:%v:%d", node.signer.Genesis.Members, node.signer.Genesis.Threshold))
		v, _ := node.store.ReadProperty(ctx, pid)
		var om map[string]any
		json.Unmarshal([]byte(v), &om)
		b, _ := hex.DecodeString(om["memo"].(string))
		b = common.AESDecrypt(node.signerAESKey[:], b)
		o, err := common.DecodeOperation(b)
		require.Nil(err)
		require.Equal(pid, o.Id)
	}
	testSpareKeys(ctx, require, node, 0, 1, 1, 1)

	for i := 0; i < 10; i++ {
		testUpdateAccountPrice(ctx, require, node)
	}
	rid, publicKey := testSafeProposeAccount(ctx, require, node, mpc, observer, accountant)
	testSpareKeys(ctx, require, node, 0, 0, 0, 0)
	testSafeApproveAccount(ctx, require, node, mpc, observer, accountant, rid, publicKey)
	testSpareKeys(ctx, require, node, 0, 0, 0, 0)
	for i := 0; i < 10; i++ {
		testUpdateNetworkStatus(ctx, require, node)
	}

	input := &bitcoin.Input{
		TransactionHash: "f04f905bb7d8e5ae1d7cd19eac8be5814380ff3fee0c4782ecb9f76dcad1e4a8",
		Index:           2, Satoshi: 21720,
	}
	testObserverAccountantDeposit(ctx, require, node, accountant, input, "0.0002172", 1)
	input = &bitcoin.Input{
		TransactionHash: "e2dd1c093457be75ba1af183c66fcfbdb042dfa91ead0d7bf3715029b756e282",
		Index:           0, Satoshi: 10000,
	}
	testObserverAccountantDeposit(ctx, require, node, accountant, input, "0.0003172", 2)

	bondId := testDeployBondContract(ctx, require, node, testSafePublicKey, SafeBitcoinChainId)
	require.Equal(testBondAssetId, bondId)
	node.ProcessOutput(ctx, &mtg.Output{AssetID: bondId, Amount: decimal.NewFromInt(1000000), CreatedAt: time.Now()})
	input = &bitcoin.Input{
		TransactionHash: "f04f905bb7d8e5ae1d7cd19eac8be5814380ff3fee0c4782ecb9f76dcad1e4a8",
		Index:           1, Satoshi: 15060,
	}
	testObserverHolderDeposit(ctx, require, node, mpc, observer, input, 1)
	input = &bitcoin.Input{
		TransactionHash: "afa34dd32333cc9d92f0deee82f8a84dfbb6db5be35bf37155189e35850d408e",
		Index:           0, Satoshi: 10000,
	}
	testObserverHolderDeposit(ctx, require, node, mpc, observer, input, 2)

	transactionHash := testSafeProposeTransaction(ctx, require, node, mpc, accountant, bondId, "3e37ea1c-1455-400d-9642-f6bbcd8c744e", "27c2c4276045678fa1f35d5506d4573d99cf1dcfe8552dae05bcb829006757c9", "002027c2c4276045678fa1f35d5506d4573d99cf1dcfe8552dae05bcb829006757c903b870736274ff0100fd3e010200000004a8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00100000000ffffffff8e400d85359e185571f35be35bdbb6fb4da8f882eedef0929dcc3323d34da3af0000000000ffffffffa8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00200000000ffffffff82e256b7295071f37b0dad1ea9df42b0bdcf6fc683f11aba75be5734091cdde20000000000ffffffff040c30000000000000220020fbf817b9dd1197a37e47af0a99b2f3ea252caf13f5ea2a18cc6bec9a1b981490d831000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e26c6b000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000000000126a103e37ea1c1455400d9642f6bbcd8c744e00000000095349474841534845538012d8b6ba5dbcf6e73299752962fb92d69489c7b83806c60f08545a08ad6036cb1424d5ac7d204fca1b059cbd15bba7c64db0a95631443488848c8572e9ecc9165af11fd5797475eb5c5bce2681611841e86429a34be9865b839a0d11238b2393a71b7033378a9810f3acf236f739eb4c487574b0979d17441fd5d040faaf6e3a0001012bd43a000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001012b1027000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001011fd854000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250001011f1027000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000000000000000107c", "70736274ff0100fd3e010200000004a8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00100000000ffffffff8e400d85359e185571f35be35bdbb6fb4da8f882eedef0929dcc3323d34da3af0000000000ffffffffa8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00200000000ffffffff82e256b7295071f37b0dad1ea9df42b0bdcf6fc683f11aba75be5734091cdde20000000000ffffffff040c30000000000000220020fbf817b9dd1197a37e47af0a99b2f3ea252caf13f5ea2a18cc6bec9a1b981490d831000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e26c6b000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000000000126a103e37ea1c1455400d9642f6bbcd8c744e00000000095349474841534845538012d8b6ba5dbcf6e73299752962fb92d69489c7b83806c60f08545a08ad6036cb1424d5ac7d204fca1b059cbd15bba7c64db0a95631443488848c8572e9ecc9165af11fd5797475eb5c5bce2681611841e86429a34be9865b839a0d11238b2393a71b7033378a9810f3acf236f739eb4c487574b0979d17441fd5d040faaf6e3a0001012bd43a000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001012b1027000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001011fd854000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250001011f1027000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000")
	testSafeRevokeTransaction(ctx, require, node, transactionHash, assert.New(t), signers)
	transactionHash = testSafeProposeTransaction(ctx, require, node, mpc, accountant, bondId, "b0a22078-0a86-459d-93f4-a1aadbf2b9b7", "e75d2f1ba953334f7d848b43ee3976cf302fb7109f09f861fedd2128d7de88a2", "0020e75d2f1ba953334f7d848b43ee3976cf302fb7109f09f861fedd2128d7de88a203b870736274ff0100fd3e010200000004a8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00100000000ffffffff8e400d85359e185571f35be35bdbb6fb4da8f882eedef0929dcc3323d34da3af0000000000ffffffffa8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00200000000ffffffff82e256b7295071f37b0dad1ea9df42b0bdcf6fc683f11aba75be5734091cdde20000000000ffffffff040c30000000000000220020fbf817b9dd1197a37e47af0a99b2f3ea252caf13f5ea2a18cc6bec9a1b981490d831000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e26c6b000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000000000126a10b0a220780a86459d93f4a1aadbf2b9b70000000009534947484153484553800b7287862377c7365dbf6a238ae10071066195bf1b79ed6bcca02071cbfe895f1440c9d5d4ba3e3030dc48e8853dfae2ba238aedf781e2b84cb577fc17089a65994c962dbbd42beabfa3b72843f4fb9c86876ed1036c8da63606e6ec3fe1da7e019f8e83cf59dd40d7703c6854a88355a5eec2726267536bee3477d13586c73b0001012bd43a000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001012b1027000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001011fd854000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250001011f1027000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000000000000000107c", "70736274ff0100fd3e010200000004a8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00100000000ffffffff8e400d85359e185571f35be35bdbb6fb4da8f882eedef0929dcc3323d34da3af0000000000ffffffffa8e4d1ca6df7b9ec82470cee3fff804381e58bac9ed17c1daee5d8b75b904ff00200000000ffffffff82e256b7295071f37b0dad1ea9df42b0bdcf6fc683f11aba75be5734091cdde20000000000ffffffff040c30000000000000220020fbf817b9dd1197a37e47af0a99b2f3ea252caf13f5ea2a18cc6bec9a1b981490d831000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e26c6b000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000000000126a10b0a220780a86459d93f4a1aadbf2b9b70000000009534947484153484553800b7287862377c7365dbf6a238ae10071066195bf1b79ed6bcca02071cbfe895f1440c9d5d4ba3e3030dc48e8853dfae2ba238aedf781e2b84cb577fc17089a65994c962dbbd42beabfa3b72843f4fb9c86876ed1036c8da63606e6ec3fe1da7e019f8e83cf59dd40d7703c6854a88355a5eec2726267536bee3477d13586c73b0001012bd43a000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001012b1027000000000000220020f64603858cd6a93e0b6300e7461311b61a76558bea58d69f76596efa0054f7e20103040100000001057463526703070040b27521028b38d2b60ab1e2391b8a469805a432dd60f7aa63c063311ce2246a78d000e514ad516821023cd0e5c08c30ae0e059ca1e27d894e609860ff6e0846637282899921b4f9b0f82102bf0a7fa4b7905a0de5ab60a5322529e1a591ddd1ee53df82e751e8adb4bed08c52ae0001011fd854000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250001011f1027000000000000160014b60581aed325e0a51cc82cd9c949902e8d4d6125010304010000000105160014b60581aed325e0a51cc82cd9c949902e8d4d61250000000000")
	testSafeApproveTransaction(ctx, require, node, transactionHash, assert.New(t), signers)
	testSpareKeys(ctx, require, node, 0, 0, 0, 0)
}

func testUpdateAccountPrice(ctx context.Context, require *require.Assertions, node *Node) {
	id := uuid.Must(uuid.NewV4()).String()

	extra := []byte{SafeChainBitcoin}
	extra = append(extra, uuid.Must(uuid.FromString(testAccountPriceAssetId)).Bytes()...)
	extra = binary.BigEndian.AppendUint64(extra, 100000000)
	extra = binary.BigEndian.AppendUint64(extra, 10000)
	dummy := testPublicKey(testBitcoinKeyDummyHolderPrivate)
	out := testBuildObserverRequest(node, id, dummy, common.ActionObserverSetAccountPlan, extra)
	testStep(ctx, require, node, out)

	plan, err := node.store.ReadAccountPlan(ctx, SafeChainBitcoin)
	require.Nil(err)
	require.Equal(testAccountPriceAssetId, plan.AccountPriceAsset)
	require.Equal("1", plan.AccountPriceAmount.String())
	require.Equal("0.0001", plan.TransactionMinimum.String())
}

func testUpdateNetworkStatus(ctx context.Context, require *require.Assertions, node *Node) {
	id := uuid.Must(uuid.NewV4()).String()
	fee, height := bitcoinMinimumFeeRate, uint64(782705)
	hash, _ := crypto.HashFromString("00000000000000000003b3730eecb8864ae6c077370d464d3044676e52d2276c")

	extra := []byte{SafeChainBitcoin}
	extra = binary.BigEndian.AppendUint64(extra, uint64(fee))
	extra = binary.BigEndian.AppendUint64(extra, height)
	extra = append(extra, hash[:]...)
	dummy := testPublicKey(testBitcoinKeyDummyHolderPrivate)
	out := testBuildObserverRequest(node, id, dummy, common.ActionObserverUpdateNetworkStatus, extra)
	testStep(ctx, require, node, out)

	info, err := node.store.ReadLatestNetworkInfo(ctx, SafeChainBitcoin)
	require.Nil(err)
	require.NotNil(info)
	require.Equal(byte(SafeChainBitcoin), info.Chain)
	require.Equal(uint64(fee), info.Fee)
	require.Equal(height, info.Height)
	require.Equal(hash.String(), info.Hash)
}

func testSafeRevokeTransaction(ctx context.Context, require *require.Assertions, node *Node, transactionHash string, assert *assert.Assertions, signers []*signer.Node) {
	id := uuid.Must(uuid.NewV4()).String()

	tx, _ := node.store.ReadTransaction(ctx, transactionHash)
	require.Equal(common.RequestStateInitial, tx.State)

	balance, err := node.store.ReadAccountantBalance(ctx, tx.Holder)
	require.Nil(err)
	require.Equal("0", balance.String())

	hb, _ := hex.DecodeString(testBitcoinKeyHolderPrivate)
	holder, _ := btcec.PrivKeyFromBytes(hb)
	msg := bitcoin.HashMessageForSignature(transactionHash)
	sig := ecdsa.Sign(holder, msg).Serialize()
	extra := uuid.Must(uuid.FromString(tx.RequestId)).Bytes()
	extra = append(extra, sig...)

	out := testBuildObserverRequest(node, id, testPublicKey(testBitcoinKeyHolderPrivate), common.ActionBitcoinSafeRevokeTransaction, extra)
	testStep(ctx, require, node, out)
	requests, err := node.store.ListAllSignaturesForTransaction(ctx, transactionHash, common.RequestStateInitial)
	require.Nil(err)
	require.Len(requests, 0)
	tx, _ = node.store.ReadTransaction(ctx, transactionHash)
	require.Equal(common.RequestStateFailed, tx.State)

	balance, err = node.store.ReadAccountantBalance(ctx, tx.Holder)
	require.Nil(err)
	require.Equal("0.0003172", balance.String())
}

func testSafeApproveTransaction(ctx context.Context, require *require.Assertions, node *Node, transactionHash string, assert *assert.Assertions, signers []*signer.Node) {
	id := uuid.Must(uuid.NewV4()).String()

	ab, _ := hex.DecodeString(testBitcoinKeyAccountantPrivate)
	accountant, _ := btcec.PrivKeyFromBytes(ab)

	tx, _ := node.store.ReadTransaction(ctx, transactionHash)
	require.Equal(common.RequestStateInitial, tx.State)
	safe, _ := node.store.ReadSafe(ctx, tx.Holder)

	hb, _ := hex.DecodeString(testBitcoinKeyHolderPrivate)
	holder, _ := btcec.PrivKeyFromBytes(hb)
	msg := bitcoin.HashMessageForSignature(transactionHash)
	sig := ecdsa.Sign(holder, msg).Serialize()
	extra := uuid.Must(uuid.FromString(tx.RequestId)).Bytes()
	extra = append(extra, sig...)

	out := testBuildObserverRequest(node, id, testPublicKey(testBitcoinKeyHolderPrivate), common.ActionBitcoinSafeApproveTransaction, extra)
	testStep(ctx, require, node, out)
	requests, err := node.store.ListAllSignaturesForTransaction(ctx, transactionHash, common.RequestStateInitial)
	require.Nil(err)
	require.Len(requests, 2)
	tx, _ = node.store.ReadTransaction(ctx, transactionHash)
	require.Equal(common.RequestStatePending, tx.State)

	msg, _ = hex.DecodeString(requests[0].Message)
	out = testBuildSignerOutput(node, requests[0].RequestId, safe.Signer, common.OperationTypeSignInput, msg)
	op := signer.TestCMPProcessOutput(ctx, assert, signers, out, requests[0].RequestId)
	out = testBuildSignerOutput(node, requests[0].RequestId, safe.Signer, common.OperationTypeSignOutput, op.Extra)
	testStep(ctx, require, node, out)
	requests, _ = node.store.ListAllSignaturesForTransaction(ctx, transactionHash, common.RequestStatePending)
	require.Len(requests, 1)
	requests, _ = node.store.ListAllSignaturesForTransaction(ctx, transactionHash, common.RequestStateInitial)
	require.Len(requests, 1)
	tx, _ = node.store.ReadTransaction(ctx, transactionHash)
	require.Equal(common.RequestStatePending, tx.State)

	msg, _ = hex.DecodeString(requests[1].Message)
	out = testBuildSignerOutput(node, requests[1].RequestId, safe.Signer, common.OperationTypeSignInput, msg)
	op = signer.TestCMPProcessOutput(ctx, assert, signers, out, requests[1].RequestId)
	out = testBuildSignerOutput(node, requests[1].RequestId, safe.Signer, common.OperationTypeSignOutput, op.Extra)
	testStep(ctx, require, node, out)
	requests, _ = node.store.ListAllSignaturesForTransaction(ctx, transactionHash, common.RequestStateInitial)
	require.Len(requests, 0)
	requests, _ = node.store.ListAllSignaturesForTransaction(ctx, transactionHash, common.RequestStatePending)
	require.Len(requests, 0)
	requests, _ = node.store.ListAllSignaturesForTransaction(ctx, transactionHash, common.RequestStateDone)
	require.Len(requests, 2)
	tx, _ = node.store.ReadTransaction(ctx, transactionHash)
	require.Equal(common.RequestStateDone, tx.State)

	signed := make(map[int][]byte)
	for _, r := range requests {
		b, _ := hex.DecodeString(r.Signature.String)
		signed[r.InputIndex] = b
	}
	mb := common.DecodeHexOrPanic(tx.RawTransaction)
	exk := common.MVMHash(mb)
	rid := mixin.UniqueConversationID(transactionHash, hex.EncodeToString(exk))
	b := testReadObserverResponse(ctx, require, node, rid, common.ActionBitcoinSafeApproveTransaction)
	assert.Equal(mb, b)

	b, _ = hex.DecodeString(tx.RawTransaction)
	psbt, _ := bitcoin.UnmarshalPartiallySignedTransaction(b)
	msgTx := psbt.Packet.UnsignedTx
	for idx := range msgTx.TxIn {
		pop := msgTx.TxIn[idx].PreviousOutPoint
		hash := psbt.SigHash(idx)
		utxo, _ := node.store.ReadBitcoinUTXO(ctx, pop.Hash.String(), int(pop.Index))
		if msig := signed[idx]; msig != nil {
			signature := ecdsa.Sign(holder, hash)
			sig := append(signature.Serialize(), byte(txscript.SigHashAll))
			msgTx.TxIn[idx].Witness = append(msgTx.TxIn[idx].Witness, []byte{})
			msgTx.TxIn[idx].Witness = append(msgTx.TxIn[idx].Witness, sig)

			msig = append(msig, byte(txscript.SigHashAll))
			der, _ := ecdsa.ParseDERSignature(msig[:len(msig)-1])
			pub, _ := hex.DecodeString(safe.Signer)
			signer, _ := btcutil.NewAddressPubKey(pub, &chaincfg.MainNetParams)
			require.True(der.Verify(hash, signer.PubKey()))
			msgTx.TxIn[idx].Witness = append(msgTx.TxIn[idx].Witness, msig)
			msgTx.TxIn[idx].Witness = append(msgTx.TxIn[idx].Witness, []byte{1})

			msgTx.TxIn[idx].Witness = append(msgTx.TxIn[idx].Witness, utxo.Script)
		} else {
			signature := ecdsa.Sign(accountant, hash)
			sig := append(signature.Serialize(), byte(txscript.SigHashAll))
			msgTx.TxIn[idx].Witness = append(msgTx.TxIn[idx].Witness, sig)
			msgTx.TxIn[idx].Witness = append(msgTx.TxIn[idx].Witness, utxo.Script)
		}
	}

	var signedBuffer bytes.Buffer
	msgTx.BtcEncode(&signedBuffer, wire.ProtocolVersion, wire.WitnessEncoding)
	logger.Println(hex.EncodeToString(signedBuffer.Bytes()))
}

func testSafeProposeTransaction(ctx context.Context, require *require.Assertions, node *Node, signer, accountant, bondId string, rid, rhash, rraw, rp string) string {
	holder := testPublicKey(testBitcoinKeyHolderPrivate)
	info, _ := node.store.ReadLatestNetworkInfo(ctx, SafeChainBitcoin)
	extra := uuid.Must(uuid.FromString(info.RequestId)).Bytes()
	extra = append(extra, []byte(testTransactionReceiver)...)
	out := testBuildHolderRequest(node, rid, holder, common.ActionBitcoinSafeProposeTransaction, bondId, extra, decimal.NewFromFloat(0.000123))
	testStep(ctx, require, node, out)

	balance, err := node.store.ReadAccountantBalance(ctx, holder)
	require.Nil(err)
	require.Equal("0", balance.String())

	b := testReadObserverResponse(ctx, require, node, rid, common.ActionBitcoinSafeProposeTransaction)
	require.Equal(rraw, hex.EncodeToString(b))
	psbt, err := bitcoin.UnmarshalPartiallySignedTransaction(b)
	require.Nil(err)
	require.Equal(rhash, psbt.Hash)
	require.Equal(int64(4220), psbt.Fee)
	var raw bytes.Buffer
	psbt.Packet.Serialize(&raw)
	require.Equal(rp, hex.EncodeToString(raw.Bytes()))

	tx := psbt.Packet.UnsignedTx
	require.Len(tx.TxOut, 4)
	main := tx.TxOut[0]
	require.Equal(int64(12300), main.Value)
	script, _ := txscript.ParsePkScript(main.PkScript)
	addr, _ := script.Address(&chaincfg.MainNetParams)
	require.Equal(testTransactionReceiver, addr.EncodeAddress())
	change := tx.TxOut[1]
	require.Equal(int64(12760), change.Value)
	script, _ = txscript.ParsePkScript(change.PkScript)
	addr, _ = script.Address(&chaincfg.MainNetParams)
	require.Equal(testSafePublicKey, addr.EncodeAddress())
	fee := tx.TxOut[2]
	require.Equal(int64(27500), fee.Value)
	script, _ = txscript.ParsePkScript(fee.PkScript)
	addr, _ = script.Address(&chaincfg.MainNetParams)
	require.Equal(testSafeAccountant, addr.EncodeAddress())

	stx, err := node.store.ReadTransaction(ctx, psbt.Hash)
	require.Nil(err)
	require.Equal(hex.EncodeToString(psbt.Marshal()), stx.RawTransaction)
	require.Equal("0.0000422", stx.Fee.String())
	require.Equal("[{\"amount\":\"0.000123\",\"receiver\":\"bc1ql0up0wwazxt6xlj84u9fnvhnagjjetcn7h4z5xxvd0kf5xuczjgqq2aehc\"}]", stx.Data)
	require.Equal(common.RequestStateInitial, stx.State)

	return stx.TransactionHash
}

func testObserverHolderDeposit(ctx context.Context, require *require.Assertions, node *Node, signer, observer string, input *bitcoin.Input, t int) {
	id := uuid.Must(uuid.NewV4()).String()
	hash, _ := crypto.HashFromString(input.TransactionHash)
	extra := []byte{SafeChainBitcoin}
	extra = append(extra, uuid.Must(uuid.FromString(SafeBitcoinChainId)).Bytes()...)
	extra = append(extra, hash[:]...)
	extra = binary.BigEndian.AppendUint64(extra, uint64(input.Index))
	extra = append(extra, big.NewInt(input.Satoshi).Bytes()...)

	holder := testPublicKey(testBitcoinKeyHolderPrivate)
	wsa, _ := bitcoin.BuildWitnessScriptAccount(holder, signer, observer, node.bitcoinTimeLockDuration(ctx))

	out := testBuildObserverRequest(node, id, holder, common.ActionObserverHolderDeposit, extra)
	testStep(ctx, require, node, out)

	balance, err := node.store.ReadAccountantBalance(ctx, holder)
	require.Nil(err)
	require.Equal("0.0003172", balance.String())

	mainInputs, feeInputs, err := node.store.ListAllBitcoinUTXOsForHolder(ctx, holder)
	require.Nil(err)
	require.Len(mainInputs, 1*t)
	require.Len(feeInputs, 2)
	utxo := mainInputs[t-1]
	require.Equal(uint32(input.Index), utxo.Index)
	require.Equal(input.Satoshi, utxo.Satoshi)
	require.Equal(wsa.Script, utxo.Script)
	require.Equal(hash.String(), utxo.TransactionHash)
	require.Equal(uint32(0x400007), wsa.Sequence)
	require.True(bitcoin.CheckMultisigHolderSignerScript(utxo.Script))
}

func testObserverAccountantDeposit(ctx context.Context, require *require.Assertions, node *Node, accountant string, input *bitcoin.Input, total string, t int) {
	id := uuid.Must(uuid.NewV4()).String()
	hash, _ := crypto.HashFromString(input.TransactionHash)
	extra := []byte{SafeChainBitcoin}
	extra = append(extra, uuid.Must(uuid.FromString(SafeBitcoinChainId)).Bytes()...)
	extra = append(extra, hash[:]...)
	extra = binary.BigEndian.AppendUint64(extra, uint64(input.Index))
	extra = append(extra, big.NewInt(input.Satoshi).Bytes()...)

	wka, _ := bitcoin.BuildWitnessKeyAccount(accountant)

	holder := testPublicKey(testBitcoinKeyHolderPrivate)
	out := testBuildObserverRequest(node, id, holder, common.ActionObserverAccountantDepost, extra)
	testStep(ctx, require, node, out)

	balance, err := node.store.ReadAccountantBalance(ctx, holder)
	require.Nil(err)
	require.Equal(total, balance.String())

	mainInputs, feeInputs, err := node.store.ListAllBitcoinUTXOsForHolder(ctx, holder)
	require.Nil(err)
	require.Len(mainInputs, 0)
	require.Len(feeInputs, 1*t)
	utxo := feeInputs[t-1]
	require.Equal(uint32(input.Index), utxo.Index)
	require.Equal(input.Satoshi, utxo.Satoshi)
	require.Equal(wka.Script, utxo.Script)
	require.Equal(hash.String(), utxo.TransactionHash)
	require.Equal(uint32(bitcoin.MaxTransactionSequence), utxo.Sequence)
	require.False(bitcoin.CheckMultisigHolderSignerScript(utxo.Script))
}

func testSafeProposeAccount(ctx context.Context, require *require.Assertions, node *Node, signer, observer, accountant string) (string, string) {
	id := uuid.Must(uuid.NewV4()).String()
	holder := testPublicKey(testBitcoinKeyHolderPrivate)
	extra := testRecipient()
	out := testBuildHolderRequest(node, id, holder, common.ActionBitcoinSafeProposeAccount, testAccountPriceAssetId, extra, decimal.NewFromInt(1))
	testStep(ctx, require, node, out)
	b := testReadObserverResponse(ctx, require, node, id, common.ActionBitcoinSafeProposeAccount)
	wsa, aaddr, err := bitcoin.UnmarshalWitnessScriptAccountWitAccountant(b)
	require.Equal(testSafePublicKey, wsa.Address)
	require.Equal(uint32(0x400007), wsa.Sequence)
	require.Equal(testSafeAccountant, aaddr)

	safe, err := node.store.ReadSafeProposal(ctx, id)
	require.Nil(err)
	require.Equal(id, safe.RequestId)
	require.Equal(holder, safe.Holder)
	require.Equal(signer, safe.Signer)
	require.Equal(observer, safe.Observer)
	require.Equal(accountant, safe.Accountant)
	public, err := bitcoin.BuildWitnessScriptAccount(holder, signer, observer, node.bitcoinTimeLockDuration(ctx))
	require.Nil(err)
	require.Equal(testSafePublicKey, public.Address)
	require.Equal(public.Address, safe.Address)
	require.Equal(byte(1), safe.Threshold)
	require.Len(safe.Receivers, 1)
	require.Equal(testSafeBondReceiverId, safe.Receivers[0])

	return id, wsa.Address
}

func testSafeApproveAccount(ctx context.Context, require *require.Assertions, node *Node, signer, observer, accountant string, rid, publicKey string) {
	id := uuid.Must(uuid.NewV4()).String()
	holder := testPublicKey(testBitcoinKeyHolderPrivate)
	hash := bitcoin.HashMessageForSignature(publicKey)
	hb, _ := hex.DecodeString(testBitcoinKeyHolderPrivate)
	hp, _ := btcec.PrivKeyFromBytes(hb)
	signature := ecdsa.Sign(hp, hash)
	extra := uuid.FromStringOrNil(rid).Bytes()
	extra = append(extra, signature.Serialize()...)
	out := testBuildObserverRequest(node, id, holder, common.ActionBitcoinSafeApproveAccount, extra)
	testStep(ctx, require, node, out)
	b := testReadObserverResponse(ctx, require, node, id, common.ActionBitcoinSafeApproveAccount)
	wsa, aaddr, err := bitcoin.UnmarshalWitnessScriptAccountWitAccountant(b)
	require.Equal(testSafePublicKey, wsa.Address)
	require.Equal(uint32(0x400007), wsa.Sequence)
	require.Equal(testSafeAccountant, aaddr)

	safe, err := node.store.ReadSafe(ctx, holder)
	require.Nil(err)
	require.Equal(id, safe.RequestId)
	require.Equal(holder, safe.Holder)
	require.Equal(signer, safe.Signer)
	require.Equal(observer, safe.Observer)
	require.Equal(accountant, safe.Accountant)
	public, err := bitcoin.BuildWitnessScriptAccount(holder, signer, observer, node.bitcoinTimeLockDuration(ctx))
	require.Nil(err)
	require.Equal(testSafePublicKey, public.Address)
	require.Equal(public.Address, safe.Address)
	require.Equal(byte(1), safe.Threshold)
	require.Len(safe.Receivers, 1)
	require.Equal(testSafeBondReceiverId, safe.Receivers[0])
}

func testStep(ctx context.Context, require *require.Assertions, node *Node, out *mtg.Output) {
	node.ProcessOutput(ctx, out)
	timestamp, err := node.timestamp(ctx)
	require.Nil(err)
	require.Equal(out.CreatedAt.UTC(), timestamp.UTC())
	req, err := node.store.ReadPendingRequest(ctx)
	require.Nil(err)
	require.NotNil(req)
	err = req.VerifyFormat()
	require.Nil(err)
	err = node.processRequest(ctx, req)
	require.Nil(err)
	req, err = node.store.ReadPendingRequest(ctx)
	require.Nil(err)
	require.Nil(req)
}

func testSpareKeys(ctx context.Context, require *require.Assertions, node *Node, hc, sc, oc, ac int) {
	for r, c := range map[int]int{
		common.RequestRoleHolder:     hc,
		common.RequestRoleSigner:     sc,
		common.RequestRoleObserver:   oc,
		common.RequestRoleAccountant: ac,
	} {
		skc, err := node.store.CountSpareKeys(ctx, common.CurveSecp256k1ECDSABitcoin, r)
		require.Nil(err)
		require.Equal(c, skc)
	}
}

func testReadObserverResponse(ctx context.Context, require *require.Assertions, node *Node, id string, typ byte) []byte {
	v, _ := node.store.ReadProperty(ctx, id)
	var om map[string]any
	json.Unmarshal([]byte(v), &om)
	require.Equal(node.conf.ObserverUserId, om["receivers"].([]any)[0])
	require.Equal(node.conf.ObserverAssetId, om["asset_id"])
	require.Equal("1", om["amount"])
	b, _ := hex.DecodeString(om["memo"].(string))
	b = common.AESDecrypt(node.observerAESKey[:], b)
	op, err := common.DecodeOperation(b)
	require.Nil(err)
	require.Equal(typ, op.Type)
	require.Equal(id, op.Id)
	require.Len(op.Extra, 32)
	v, _ = node.store.ReadProperty(ctx, hex.EncodeToString(op.Extra))
	b, _ = hex.DecodeString(v)
	return b
}

func testBuildHolderRequest(node *Node, id, public string, action byte, assetId string, extra []byte, amount decimal.Decimal) *mtg.Output {
	op := &common.Operation{
		Id:     id,
		Type:   action,
		Curve:  common.CurveSecp256k1ECDSABitcoin,
		Public: public,
		Extra:  extra,
	}
	memo := base64.RawURLEncoding.EncodeToString(op.Encode())
	return &mtg.Output{
		AssetID:         assetId,
		Memo:            memo,
		TransactionHash: crypto.NewHash([]byte(op.Id)),
		Amount:          amount,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func testBuildObserverRequest(node *Node, id, public string, action byte, extra []byte) *mtg.Output {
	op := &common.Operation{
		Id:     id,
		Type:   action,
		Curve:  common.CurveSecp256k1ECDSABitcoin,
		Public: public,
		Extra:  extra,
	}
	memo := common.Base91Encode(node.encryptObserverOperation(op))
	return &mtg.Output{
		Sender:          node.conf.ObserverUserId,
		AssetID:         node.conf.ObserverAssetId,
		Memo:            memo,
		TransactionHash: crypto.NewHash([]byte(op.Id)),
		Amount:          decimal.New(1, 1),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func testBuildSignerOutput(node *Node, id, public string, action byte, extra []byte) *mtg.Output {
	op := &common.Operation{
		Id:    id,
		Type:  action,
		Curve: common.CurveSecp256k1ECDSABitcoin,
		Extra: extra,
	}
	switch action {
	case common.OperationTypeKeygenInput:
		op.Public = hex.EncodeToString(common.ShortSum(public))
	case common.OperationTypeSignInput:
		op.Public = hex.EncodeToString(common.ShortSum(public))
	case common.OperationTypeKeygenOutput:
		op.Public = public
	case common.OperationTypeSignOutput:
		op.Public = public
	}
	memo := mtg.EncodeMixinExtra("", id, string(node.encryptSignerOperation(op)))
	return &mtg.Output{
		AssetID:         node.conf.AssetId,
		Memo:            memo,
		TransactionHash: crypto.NewHash([]byte(op.Id)),
		Amount:          decimal.New(1, 1),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func testDeployBondContract(ctx context.Context, require *require.Assertions, node *Node, addr, assetId string) string {
	safe, _ := node.store.ReadSafeByAddress(ctx, addr)
	asset, _ := node.fetchAssetMeta(ctx, assetId)
	err := abi.GetOrDeployFactoryAsset("https://geth.mvm.dev", os.Getenv("MVM_DEPLOYER"), asset.AssetId, asset.Symbol, asset.Name, safe.Holder)
	require.Nil(err)
	bond := abi.GetFactoryAssetAddress(assetId, asset.Symbol, asset.Name, safe.Holder)
	assetKey := strings.ToLower(bond.String())
	err = mvm.VerifyAssetKey(assetKey)
	require.Nil(err)
	asset, _ = node.fetchAssetMeta(ctx, mvm.GenerateAssetId(assetKey).String())
	return asset.AssetId
}

func testBuildNode(ctx context.Context, require *require.Assertions, root string) *Node {
	f, _ := os.ReadFile("../config/example.toml")
	var conf struct {
		Keeper *Configuration `toml:"keeper"`
		Signer struct {
			MTG *mtg.Configuration `toml:"mtg"`
		} `toml:"signer"`
	}
	err := toml.Unmarshal(f, &conf)
	require.Nil(err)

	conf.Keeper.StoreDir = root
	if !strings.HasPrefix(conf.Keeper.StoreDir, "/tmp/") {
		panic(root)
	}
	kd, err := OpenSQLite3Store(conf.Keeper.StoreDir + "/safe.sqlite3")
	require.Nil(err)

	db, err := store.OpenBadger(ctx, conf.Keeper.StoreDir+"/mtg")
	require.Nil(err)
	group, err := mtg.BuildGroup(ctx, db, conf.Keeper.MTG)
	require.NotNil(err)
	require.Nil(group)

	node := NewNode(kd, group, conf.Keeper, conf.Signer.MTG)
	return node
}

func testRecipient() []byte {
	extra := []byte{1, 1}
	id := uuid.FromStringOrNil(testSafeBondReceiverId)
	return append(extra, id.Bytes()...)
}

func testPublicKey(pub string) string {
	seed, _ := hex.DecodeString(pub)
	_, dk := btcec.PrivKeyFromBytes(seed)
	return hex.EncodeToString(dk.SerializeCompressed())
}
