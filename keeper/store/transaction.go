package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/MixinNetwork/safe/apps/bitcoin"
	"github.com/MixinNetwork/safe/common"
	"github.com/shopspring/decimal"
)

type Transaction struct {
	TransactionHash string
	RawTransaction  string
	Holder          string
	Chain           byte
	State           int
	Fee             decimal.Decimal
	RequestId       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

var transactionCols = []string{"transaction_hash", "raw_transaction", "holder", "chain", "state", "fee", "request_id", "created_at", "updated_at"}

func (s *SQLite3Store) ReadTransactionByRequestId(ctx context.Context, requestId string) (*Transaction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var hash string
	query := "SELECT transaction_hash FROM transactions WHERE request_id=?"
	row := tx.QueryRowContext(ctx, query, requestId)
	err = row.Scan(&hash)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return s.readTransaction(ctx, tx, hash)
}

func (s *SQLite3Store) ReadTransaction(ctx context.Context, hash string) (*Transaction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return s.readTransaction(ctx, tx, hash)
}

func (s *SQLite3Store) WriteTransactionWithRequest(ctx context.Context, trx *Transaction, utxos []*bitcoin.Input, spend decimal.Decimal) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	feeBalance, err := s.readAccountantBalance(ctx, tx, trx.Holder)
	if err != nil {
		return err
	}
	feeBalance = feeBalance.Sub(spend)
	if feeBalance.IsNegative() {
		panic(trx.RequestId)
	}

	vals := []any{trx.TransactionHash, trx.RawTransaction, trx.Holder, trx.Chain, trx.State, trx.Fee.String(), trx.RequestId, trx.CreatedAt, trx.UpdatedAt}
	err = s.execOne(ctx, tx, buildInsertionSQL("transactions", transactionCols), vals...)
	if err != nil {
		return fmt.Errorf("INSERT transactions %v", err)
	}
	err = s.execOne(ctx, tx, "UPDATE accountants SET balance=?, updated_at=? WHERE holder=?",
		feeBalance, trx.UpdatedAt, trx.Holder)
	if err != nil {
		return fmt.Errorf("UPDATE accountants %v", err)
	}
	err = s.execOne(ctx, tx, "UPDATE requests SET state=?, updated_at=? WHERE request_id=?",
		common.RequestStateDone, time.Now().UTC(), trx.RequestId)
	if err != nil {
		return fmt.Errorf("UPDATE requests %v", err)
	}
	for _, utxo := range utxos {
		err = s.execOne(ctx, tx, "UPDATE bitcoin_outputs SET state=?, spent_by=?, updated_at=? WHERE transaction_hash=? AND output_index=?",
			common.RequestStatePending, trx.TransactionHash, trx.UpdatedAt, utxo.TransactionHash, utxo.Index)
		if err != nil {
			return fmt.Errorf("UPDATE bitcoin_outputs %v", err)
		}
	}
	return tx.Commit()
}

func (s *SQLite3Store) readTransaction(ctx context.Context, tx *sql.Tx, transactionHash string) (*Transaction, error) {
	query := fmt.Sprintf("SELECT %s FROM transactions WHERE transaction_hash=?", strings.Join(transactionCols, ","))
	row := tx.QueryRowContext(ctx, query, transactionHash)

	var trx Transaction
	err := row.Scan(&trx.TransactionHash, &trx.RawTransaction, &trx.Holder, &trx.Chain, &trx.State, &trx.Fee, &trx.RequestId, &trx.CreatedAt, &trx.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &trx, err
}
