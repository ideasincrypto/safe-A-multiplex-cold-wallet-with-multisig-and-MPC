# Mixin Safe

Mixin Safe is an advanced non-custodial solution that offers a multiplex cold wallet with multisig and MPC to provide the most secure and convenient self-custody solution for cryptocurrencies. With a firm belief in "not your keys not your coins," Mixin Safe aims to address the pressing need for securing crypto keys.

Through the native Bitcoin multisig and timelock script, Mixin Safe offers a 2/3 multisig that comprises three keys, namely holder, signer, and observer. The BTC locked within the script can be spent only when both the holder and signer keys sign a transaction, provided that the timelock of one year is in effect. In case of key loss by the holder or signer, the observer can act as a rescuer after one year.

The MPC-generated signer key is controlled in a decentralized manner, ensuring added security to the user's keys. When a deposit is made into a safe account, Mixin Safe issues an equivalent amount of safeBTC to the account owner. To initiate a transaction with the holder key, the user needs to send safeBTC to the Mixin Safe network and sign the raw transaction with the holder key, thereby enabling the signer to sign the transaction together with the holder key.


## Prepare Holder Key

Currently, there aren't many Bitcoin wallets that can perform custom script signing, not even the bitcoin-core wallet. It's therefore recommended to use btcd, which can be accessed at https://github.com/btcsuite/btcd.

Using btcd, you can generate a public and private key pair using the following code:

```golang
priv, pub := btcec.PrivKeyFromBytes(seed)
fmt.Printf("public: %x\nprivate: %x\n", pub.SerializeCompressed(), priv.Serialize())

==>
public: 039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40
private: 1b639e995830c253eb38780480440a72919f5448be345a574c545329f2df4d76
```

After generating the key pair, you will need to create a random UUID as the session ID. As example, the UUID `2e78d04a-e61a-442d-a014-dec19bd61cfe` will be used.


## Propose Safe Account

All messages to the safe network should be encoded as the following operation struct:

```golang
type Operation struct {
	Id     string
	Type   uint8
	Curve  uint8
	Public string
	Extra  []byte
}

func (o *Operation) Encode() []byte {
	pub, err := hex.DecodeString(o.Public)
	if err != nil {
		panic(o.Public)
	}
	enc := common.NewEncoder()
	writeUUID(enc, o.Id)
	writeByte(enc, o.Type)
	writeByte(enc, o.Curve)
	writeBytes(enc, pub)
	writeBytes(enc, o.Extra)
	return enc.Bytes()
}
```

To send the account proposal, with the holder prepared from last step, the operation value should be like:

```golang
op := &Operation {
  Id: "2e78d04a-e61a-442d-a014-dec19bd61cfe",
  Type: 110,
  Curve: 1,
  Public: "039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40",
}
```

All above four fields above are mandatory for all safe network transactions, now we need to make the extra:

```golang
threshold := byte(1)
total := byte(1)
owners := []string{"fcb87491-4fa0-4c2f-b387-262b63cbc112"}
extra := []byte{threshold, total}
uid := uuid.FromStringOrNil(owners[0])
op.Extra = append(extra, uid.Bytes()...)
```

So the safe account proposal operation extra is encoded with threshold, owners count, and all owner UUIDs.

Then we can encode the operation and use it as a memo to send the account proposal transaction to safe network MTG:

```golang
memo := base64.RawURLEncoding.EncodeToString(op.Encode())
input := mixin.TransferInput{
  AssetID: "c6d0c728-2624-429b-8e0d-d9d19b6592fa",
  Amount:  decimal.NewFromFloat(0.0001),
  TraceID: op.Id,
  Memo:    memo,
}
input.OpponentMultisig.Receivers = []{
  "71b72e67-3636-473a-9ee4-db7ba3094057",
  "148e696f-f1db-4472-a907-ceea50c5cfde",
  "c9a9a719-4679-4057-bcf0-98945ed95a81",
  "b45dcee0-23d7-4ad1-b51e-c681a257c13e",
  "fcb87491-4fa0-4c2f-b387-262b63cbc112",
}
input.OpponentMultisig.Threshold = 4
```


## Approve Safe Account

After the account proposal transaction sent to the safe network MTG, you can monitor the Mixin Network transactions to decode your account details. But to make it easy, it's possible to just fetch it from the Safe HTTP API with the proposal UUID:

```
curl https://safe.mixin.dev/accounts/2e78d04a-e61a-442d-a014-dec19bd61cfe
{"accountant":"bc1qevu9qqpfqp4s9jq3xxulfh08rgyjy8rn76aj7e","address":"bc1qzccxhrlm4p5l5rpgnns58862ckmsat7uxucqjfcfmg7ef6yltf3quhr94a","id":"2e78d04a-e61a-442d-a014-dec19bd61cfe","script":"6352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae","status":"proposed"}
```

You should have noticed that the request was made with the same session UUID we prepared at the first step. That address returned is our safe account address to receive BTC, but before using it, we must approve it with our holder key:


```golang
var buf bytes.Buffer
_ = wire.WriteVarString(&buf, 0, "Bitcoin Signed Message:\n")
_ = wire.WriteVarString(&buf, 0, address)
hash := chainhash.DoubleHashB(buf.Bytes())
b, _ := hex.DecodeString(priv)
private, _ := btcec.PrivKeyFromBytes(b)
sig := ecdsa.Sign(private, hash)
fmt.Println(base64.RawURLEncoding.EncodeToString(sig.Serialize()))

==>
MEUCIQCY3Gl1uocJR-qa2wVUuvK_gc-pOxzk8Zq_x_Hqv8iJbAIgXPbMuk-GiGsM3MJKmQ3haRzfDEKSBHArkgRF2NtxDOk
```

With the signature we send the request to safe network to prove that we own the holder key exactly:

```
curl https://safe.mixin.dev/accounts/2e78d04a-e61a-442d-a014-dec19bd61cfe -H 'Content-Type:application/json' \
  -d '{"address":"bc1qzccxhrlm4p5l5rpgnns58862ckmsat7uxucqjfcfmg7ef6yltf3quhr94a","signature":"MEUCIQCY3Gl1uocJR-qa2wVUuvK_gc-pOxzk8Zq_x_Hqv8iJbAIgXPbMuk-GiGsM3MJKmQ3haRzfDEKSBHArkgRF2NtxDOk"}'
{"accountant":"bc1qevu9qqpfqp4s9jq3xxulfh08rgyjy8rn76aj7e","address":"bc1qzccxhrlm4p5l5rpgnns58862ckmsat7uxucqjfcfmg7ef6yltf3quhr94a","id":"2e78d04a-e61a-442d-a014-dec19bd61cfe","script":"6352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae","status":"proposed"}
```

Now we can deposit BTC to the address above, and you will receive safeBTC to the owner wallet.


## Propose Safe Transaction

After deposited some BTC to both the safe address and accountant, now we want to send 0.000123BTC to bc1qevu9qqpfqp4s9jq3xxulfh08rgyjy8rn76aj7e.

First we need to generate another random session id, and `36c2075c-5af0-4593-b156-e72f58f9f421` will be used as example.

Then with the holder prepared from first step, the operation value should be like:

```golang
op := &Operation {
  Id: "36c2075c-5af0-4593-b156-e72f58f9f421",
  Type: 112,
  Curve: 1,
  Public: "039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40",
  Extra: []byte("bc1qevu9qqpfqp4s9jq3xxulfh08rgyjy8rn76aj7e"),
}
```

Then we get the safeBTC asset id we received when deposit BTC to the safe address, all safe accounts have different safe assets, the safeBTC asset id of the safe account that we created is `94683442-3ae2-3118-bec7-069c934668c0`. Use that to make a transaction to the Safe Network MTG:

```golang
memo := base64.RawURLEncoding.EncodeToString(op.Encode())
input := mixin.TransferInput{
  AssetID: "94683442-3ae2-3118-bec7-069c934668c0",
  Amount:  decimal.NewFromFloat(0.000123),
  TraceID: op.Id,
  Memo:    memo,
}
input.OpponentMultisig.Receivers = []{
  "71b72e67-3636-473a-9ee4-db7ba3094057",
  "148e696f-f1db-4472-a907-ceea50c5cfde",
  "c9a9a719-4679-4057-bcf0-98945ed95a81",
  "b45dcee0-23d7-4ad1-b51e-c681a257c13e",
  "fcb87491-4fa0-4c2f-b387-262b63cbc112",
}
input.OpponentMultisig.Threshold = 4
```

After the transaction sent successfully to the safe network MTG, we can query the safe API to get the proposed raw transaction:

```
curl https://safe.mixin.dev/transactions/36c2075c-5af0-4593-b156-e72f58f9f421
{"fee":"0.00032181","hash":"0e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f","id":"36c2075c-5af0-4593-b156-e72f58f9f421","raw":"00200e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f059e70736274ff0100fd9201020000000783ee84a3805e2ef99224bbb9fddd224571899251ce5f04600daff82755d4cbe70000000000ffffffff071a0ae95fbee6630d9f2e152c47f4f5d3656566719ddafffa9db12a39648da30000000000ffffffff309816a2e9053e59a090971b4064c1136f2a2936b2a3c8dbb5ce137766577a2b0000000000ffffffff11e022ec883c9e103cb22bf69ee955236d1343c2fe6ab3c5ef322128c8dfb03a0000000000ffffffff9addc81d72b56fbb0421f6ffbf42a4cef06e5997997d86f8531030610873a4800000000000ffffffff4231093010fe291636141725cb10401978cb2c69327f504b6fbc41af8dfec1390000000000fffffffff048cb1a26d1a843ee96e51438d5ea0337c4a7e7d5d30ac77d2dff91fe18bd730000000000ffffffff030c30000000000000160014cb38500029006b02c81131b9f4dde71a09221c732e5f00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a6231e6000000000000160014cb38500029006b02c81131b9f4dde71a09221c730000000009534947484153484553e02aa0db069d5a782dfddb0abeffbeef9c5f5413ad9c67f869946d94b33a2c639e5ceb6f138580cae26f37ea63a5ba53154d60315c530670ecce024bb66e2c9c0d956f31c985423b7059bcdb755795b794882f2d6327fef70771047b1163f541477ad45b470bd3ec2377ae40ac240050afcbaaf0f1ed83370218722081be66617c99b05cdbc3adbf55ff702d1a529a2329478425e69a429cb7fcb813edd93e7c41b2f0a4ac60de4e7f4712d6fc9fdb0ea48e74a5c0956dd7f22dbd64921433bce373d6dc067eb83718c11b802b2899a10531ab95054bd8e0eac3309cd8e5108d880001012bbe2f00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a62010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001012b153400000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a62010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001012b672b00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a62010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001011f255b000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011f7c5f000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011fce56000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011f7752000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c73000000000000000000007db5"}
```


## Approve Safe Transaction

With the transaction proposed in previous step, we can decode the raw response to get the partially signed bitcoin transaction for the holder key to sign:

```golang
b, _ := hex.DecodeString(raw)
dec := common.NewDecoder(b)
hash, _ := dec.ReadBytes()
psbtBytes, _ := dec.ReadBytes()
fee, _ := dec.ReadUint64()
```

With the decoded PSBT, we can parse it to see all its inputs and outputs, and verify it's correct as we proposed. Then we sign all signature hashes with our holder private key:

```golang
script := theSafeAccountScript()
pkt, _ = psbt.NewFromRawBytes(bytes.NewReader(psbtBytes), false)
msgTx := pkt.UnsignedTx
for idx := range msgTx.TxIn {
	hash := sigHash(pkt, idx)
	sig := ecdsa.Sign(holder, hash).Serialize()
	pkt.Inputs[idx].PartialSigs = []*psbt.PartialSig{{
		PubKey:    holder.PubKey().SerializeCompressed(),
		Signature: sig,
	}}
}
raw := marshal(hash, pkt, fee)
fmt.Printf("raw: %x\n", raw)

var buf bytes.Buffer
_ = wire.WriteVarString(&buf, 0, "Bitcoin Signed Message:\n")
_ = wire.WriteVarString(&buf, 0, msgTx.TxHash().String())
hash := chainhash.DoubleHashB(buf.Bytes())
sig := ecdsa.Sign(holder, msg).Serialize()
fmt.Printf("signature: %s\n", base64.RawURLEncoding.EncodeToString(sig))
```

After we have the PSBT signed by holder private key, then we can send them to safe API:

```
curl https://safe.mixin.dev/transactions/36c2075c-5af0-4593-b156-e72f58f9f421 -H 'Content-Type:application/json' \
  -d '{"action":"approve","chain":1,"raw":"00200e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f088870736274ff0100fd9201020000000783ee84a3805e2ef99224bbb9fddd224571899251ce5f04600daff82755d4cbe70000000000ffffffff071a0ae95fbee6630d9f2e152c47f4f5d3656566719ddafffa9db12a39648da30000000000ffffffff309816a2e9053e59a090971b4064c1136f2a2936b2a3c8dbb5ce137766577a2b0000000000ffffffff11e022ec883c9e103cb22bf69ee955236d1343c2fe6ab3c5ef322128c8dfb03a0000000000ffffffff9addc81d72b56fbb0421f6ffbf42a4cef06e5997997d86f8531030610873a4800000000000ffffffff4231093010fe291636141725cb10401978cb2c69327f504b6fbc41af8dfec1390000000000fffffffff048cb1a26d1a843ee96e51438d5ea0337c4a7e7d5d30ac77d2dff91fe18bd730000000000ffffffff030c30000000000000160014cb38500029006b02c81131b9f4dde71a09221c732e5f00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a6231e6000000000000160014cb38500029006b02c81131b9f4dde71a09221c730000000009534947484153484553e02aa0db069d5a782dfddb0abeffbeef9c5f5413ad9c67f869946d94b33a2c639e5ceb6f138580cae26f37ea63a5ba53154d60315c530670ecce024bb66e2c9c0d956f31c985423b7059bcdb755795b794882f2d6327fef70771047b1163f541477ad45b470bd3ec2377ae40ac240050afcbaaf0f1ed83370218722081be66617c99b05cdbc3adbf55ff702d1a529a2329478425e69a429cb7fcb813edd93e7c41b2f0a4ac60de4e7f4712d6fc9fdb0ea48e74a5c0956dd7f22dbd64921433bce373d6dc067eb83718c11b802b2899a10531ab95054bd8e0eac3309cd8e5108d880001012bbe2f00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a622202039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40473045022100d884c996daedf2f16b86ed77f6cbf17b58ebd579be57a1b5f6587448d7af92dc022004eb359c90f4784e316334811a47d2588c34b175d3d99c1f2a1b7ca864271537010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001012b153400000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a622202039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab404730450221008f650cf7899d2b62a79ab73dac954679809e016fcd68ccd845f32cb90e6fdbbf02202c0d31e8f97dda99d67b80a0b05b25b7ddba19c0d3f2a97f5266a9abcf9a2f6a010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001012b672b00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a622202039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40473045022100b80d83331ac14dfa0b6a0fe29fb3c9c9993dddb6c54f38047a696160be03ccd402205ca81590be93a5ca07d5e5c92ff011017645e07fe4c429734d57cb7d28267e9f010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001011f255b000000000000160014cb38500029006b02c81131b9f4dde71a09221c732202039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab4046304402201e2c1a773cf6a2df3079de33b612cfcea63f56f171de417d1eaf8f8ac240240e022018baa0ed589bfaab9caaa23acf471c24d7ee1b95a5a8334646dad3a5d26bcfb3010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011f7c5f000000000000160014cb38500029006b02c81131b9f4dde71a09221c732202039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab4047304502210096484cd1d9f0119870a8ef4ca66b20a8e1826783bd10489c1b71041666df0a3802205d57af62207860d8290fee9413da780506028963ac61882138a056f40318b734010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011fce56000000000000160014cb38500029006b02c81131b9f4dde71a09221c732202039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40463044022062bc8b13fff1d895b7f30a3f9954ca635e37d98bc4c10667cc0ebabaec88250b02202e54e829d64cd9440bc521636620b42ad71694c9565ff6330da0a6a6e240a5ec010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011f7752000000000000160014cb38500029006b02c81131b9f4dde71a09221c732202039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab404630440220241049a9092fcb7e784b7c4b8ca274e1d697d0b30fa330e18f64e2a394cd6d5902204d4273b90b8142a7f1192adbe1c4130e29ebcc2c2164fa43e915cb4e35fe7fd7010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c73000000000000000000007db5","signature":"MEQCIDfROpqb2l5b9LD5RL865HsSDvKhSGI9a6RShQwdfI9jAiBWLep5ogVplOsBETaALGtlN6GmcHIASV_nU-AUhtN0mQ"}'
{"chain":1,"fee":"0.00032181","hash":"0e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f","id":"36c2075c-5af0-4593-b156-e72f58f9f421","raw":"00200e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f059e70736274ff0100fd9201020000000783ee84a3805e2ef99224bbb9fddd224571899251ce5f04600daff82755d4cbe70000000000ffffffff071a0ae95fbee6630d9f2e152c47f4f5d3656566719ddafffa9db12a39648da30000000000ffffffff309816a2e9053e59a090971b4064c1136f2a2936b2a3c8dbb5ce137766577a2b0000000000ffffffff11e022ec883c9e103cb22bf69ee955236d1343c2fe6ab3c5ef322128c8dfb03a0000000000ffffffff9addc81d72b56fbb0421f6ffbf42a4cef06e5997997d86f8531030610873a4800000000000ffffffff4231093010fe291636141725cb10401978cb2c69327f504b6fbc41af8dfec1390000000000fffffffff048cb1a26d1a843ee96e51438d5ea0337c4a7e7d5d30ac77d2dff91fe18bd730000000000ffffffff030c30000000000000160014cb38500029006b02c81131b9f4dde71a09221c732e5f00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a6231e6000000000000160014cb38500029006b02c81131b9f4dde71a09221c730000000009534947484153484553e02aa0db069d5a782dfddb0abeffbeef9c5f5413ad9c67f869946d94b33a2c639e5ceb6f138580cae26f37ea63a5ba53154d60315c530670ecce024bb66e2c9c0d956f31c985423b7059bcdb755795b794882f2d6327fef70771047b1163f541477ad45b470bd3ec2377ae40ac240050afcbaaf0f1ed83370218722081be66617c99b05cdbc3adbf55ff702d1a529a2329478425e69a429cb7fcb813edd93e7c41b2f0a4ac60de4e7f4712d6fc9fdb0ea48e74a5c0956dd7f22dbd64921433bce373d6dc067eb83718c11b802b2899a10531ab95054bd8e0eac3309cd8e5108d880001012bbe2f00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a62010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001012b153400000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a62010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001012b672b00000000000022002016306b8ffba869fa0c289ce1439f4ac5b70eafdc3730092709da3d94e89f5a62010304010000000105746352670399f040b2752102b4868f0800a8268ea24e0ba96c61d251ec199275b955cd48fb9af2302ef250f2ad516821039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab402102c4f8174c09969f7e37ae0d2d1cb02d945625595054cf8d6fff05e0d96e9e0bc052ae0001011f255b000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011f7c5f000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011fce56000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c730001011f7752000000000000160014cb38500029006b02c81131b9f4dde71a09221c73010304010000000105160014cb38500029006b02c81131b9f4dde71a09221c73000000000000000000007db5"}
```

A few minutes later, we should be able to query the transaction on a Bitcoin explorer.

https://blockstream.info/tx/0e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f?expand
