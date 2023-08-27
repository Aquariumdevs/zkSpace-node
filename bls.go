// bls.go
package main

import (
	"crypto/sha256"

	blst "github.com/supranational/blst/bindings/go"
)

type Signature = blst.P2Affine

type PublicKey = blst.P1Affine

//type AggregateSignature = blst.P2Aggregate
//type AggregatePublicKey = blst.P1Aggregate

func (tx *Transaction) blsCompressedVerify(sig *Signature, sg, blspk, msg, dst []byte) bool {
	logs.log("Verifying Bls...")
	if sig.VerifyCompressed(sg, true, blspk, true, msg, dst) {
		logs.log("Pop Valid!")
		return true
	} else {
		logs.log("fuck")
		return false
	}
}

func (tx *Transaction) verifyBlsTx(app *App) bool {
	if tx.isAccountCreator {
		//fetch proof of posession and the public key
		tx.pop = tx.data[88:]
		tx.blspk = tx.data[40:88]

		return tx.verifyTxPop(app)
	}

	if tx.isAccountKeyChanger {
		//fetch proof of posession and the public key
		tx.pop = tx.data[84:]
		tx.blspk = tx.data[36:84]

		return tx.verifyTxPop(app)
	}

	if tx.isBatch {
		tx.multisignature = tx.data[9:73]
		tx.state = tx.data[73:113]
		return tx.verifyBatch(app)
	}

	return true
}

func (tx *Transaction) verifyTxPop(app *App) bool {
	var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_")

	//hash mecessary tx data
	h := sha256.New()
	h.Write(tx.blspk)
	h.Write(tx.source)
	h.Write(tx.counter)
	hash := h.Sum(nil)

	return tx.blsCompressedVerify(app.dummySig, tx.pop, tx.blspk, hash, dst)
}
