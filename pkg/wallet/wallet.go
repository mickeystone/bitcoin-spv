package wallet

import (
	"bytes"
	"log"
	"sort"

	"github.com/tomokazukozuma/bitcoin-spv/pkg/protocol/common"
	"github.com/tomokazukozuma/bitcoin-spv/pkg/protocol/message"
	"github.com/tomokazukozuma/bitcoin-spv/pkg/script"
	"github.com/tomokazukozuma/bitcoin-spv/pkg/util"
)

type Wallet struct {
	Key   *util.Key
	Utxos []*message.Utxo
}

func NewWallet() *Wallet {
	return &Wallet{
		Key:   util.NewKey(),
		Utxos: []*message.Utxo{},
	}
}

func (w *Wallet) GetPublicKey() []byte {
	return w.Key.PublicKey.SerializeUncompressed()
}

func (w *Wallet) GetPublicKeyHash() []byte {
	return util.Hash160(w.GetPublicKey())
}

func (w *Wallet) GetAddress() string {
	return util.EncodeAddress(w.GetPublicKey())
}

func (w *Wallet) AddUtxo(utxo *message.Utxo) {
	alreadyExists := false
	for _, u := range w.Utxos {
		if u.Hash == utxo.Hash && u.N == utxo.N {
			alreadyExists = true
		}
	}
	// TODO TxInで使われていないかチェック
	if alreadyExists {
		return
	}
	w.Utxos = append(w.Utxos, utxo)
}

func (w *Wallet) GetBalance() uint64 {
	var balance uint64
	for _, v := range w.Utxos {
		balance += v.TxOut.Value
	}
	return balance
}

func (w *Wallet) CreateTx(toAddress string, value uint64) *message.Tx {
	utxos, totalValue := w.getEnoughUtxos(value)
	fee := util.CalculateFee(10, len(utxos))
	chargeValue := totalValue - value - fee
	txouts := w.CreateTxOuts(toAddress, value, chargeValue)
	txins, err := w.CreateTxIns(utxos, txouts)
	if err != nil {
		log.Fatalf("createTxIn: %+v", err)
	}
	for _, utxo := range utxos {
		w.removeUtxo(utxo)
	}
	return message.NewTx(uint32(1), txins, txouts, uint32(0)).(*message.Tx)
}

func (w *Wallet) getEnoughUtxos(value uint64) (utxos []*message.Utxo, totalVAlue uint64) {
	sort.Slice(w.Utxos, func(i, j int) bool { return w.Utxos[i].TxOut.Value > w.Utxos[j].TxOut.Value })
	for _, utxo := range w.Utxos {
		utxos = append(utxos, utxo)
		totalVAlue += utxo.TxOut.Value
		if value <= totalVAlue {
			return
		}
	}
	return
}

func (w *Wallet) removeUtxo(u *message.Utxo) {
	var newUtxos []*message.Utxo
	for _, utxo := range w.Utxos {
		if u.Hash != utxo.Hash && u.N != utxo.N {
			newUtxos = append(newUtxos, utxo)
		}
	}
	w.Utxos = newUtxos
}

func (w *Wallet) CreateTxOuts(toAddress string, value, chargeValue uint64) []*message.TxOut {
	var txout []*message.TxOut
	lockingScript1 := script.CreateLockingScriptForPKH(util.DecodeAddress(toAddress))
	txout = append(txout, &message.TxOut{
		Value:         value,
		LockingScript: common.NewVarStr(lockingScript1),
	})

	lockingScript2 := script.CreateLockingScriptForPKH(util.DecodeAddress(w.GetAddress()))
	txout = append(txout, &message.TxOut{
		Value:         chargeValue,
		LockingScript: common.NewVarStr(lockingScript2),
	})
	return txout
}

func (w *Wallet) CreateTxIns(utxos []*message.Utxo, txouts []*message.TxOut) ([]*message.TxIn, error) {
	var txins []*message.TxIn
	for _, utxo := range utxos {

		outpoint := &message.OutPoint{
			Hash: utxo.Hash,
			N:    utxo.N,
		}

		var tmpTxins []*message.TxIn
		for _, otherUtxo := range utxos {
			if utxo.Hash == otherUtxo.Hash && utxo.N == otherUtxo.N {
				tmpTxins = append(tmpTxins, &message.TxIn{
					PreviousOutput:  outpoint,
					UnlockingScript: otherUtxo.TxOut.LockingScript,
					Sequence:        0xFFFFFFFF,
				})
			} else {
				otherOutpoint := &message.OutPoint{
					Hash: otherUtxo.Hash,
					N:    otherUtxo.N,
				}
				tmpTxins = append(tmpTxins, &message.TxIn{
					PreviousOutput:  otherOutpoint,
					UnlockingScript: common.NewVarStr([]byte{}),
					Sequence:        0xFFFFFFFF,
				})
			}
		}
		tmpTx := message.NewTx(
			uint32(1),
			tmpTxins,
			txouts,
			uint32(0),
		)

		var sigHashCode = []byte{0x01, 0x00, 0x00, 0x00} // sig hash all
		sigbatureHash := util.Hash256(bytes.Join([][]byte{
			tmpTx.Encode(),
			sigHashCode,
		}, []byte{}))

		signature, err := w.Sign(sigbatureHash)
		if err != nil {
			return nil, err
		}
		log.Printf("signature len: %+v", len(signature))
		var sigHashType = []byte{0x01}
		signatureWithType := bytes.Join([][]byte{signature, sigHashType}, []byte{})
		txin := &message.TxIn{
			PreviousOutput:  outpoint,
			UnlockingScript: script.CreateUnlockingScriptForPKH(signatureWithType, w.GetPublicKey()),
			Sequence:        0xFFFFFFFF,
		}
		txins = append(txins, txin)
	}
	log.Printf("==== txins len: %+v", len(txins))
	return txins, nil
}

func (w *Wallet) Sign(sigHash []byte) ([]byte, error) {
	return w.Key.Sign(sigHash)
}
