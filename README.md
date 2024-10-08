# Mixin Safe

Mixin Safe is an advanced non-custodial solution that specializes in providing a multiplex cold wallet. The platform places strong emphasis on the significance of securing crypto keys, with its firm belief in the principle of "not your keys, not your coins". Mixin Safe leverages mature technologies such as multisig and MPC to offer its users the most secure and convenient solution for securing crypto keys.

Through the native Bitcoin multisig and timelock script, Mixin Safe offers a 2/3 multisig that comprises three keys, namely owner, members, and recovery. The BTC locked within the script can be spent only when both the owner and members keys sign a transaction, provided that the timelock of one year is in effect. In case of key loss by the owner or members, the recovery key can act as a rescuer after the timelock expired.

A Mixin Safe account is represented by the miniscript below. Miniscript is a language for writing (a subset of) Bitcoin Scripts in a structured way, enabling analysis, composition, generic signing and more. Thus Mixin Safe is able to work with all popular Bitcoin software wallets and hardware wallets, e.g. Bitcoin Core, Ledger.

```
thresh(2,pk(OWNER),s:pk(MEMBERS),sj:and_v(v:pk(RECOVERY),n:older(52560)))

OWNER OP_CHECKSIG OP_SWAP MEMBERS OP_CHECKSIG OP_ADD OP_SWAP OP_SIZE
OP_0NOTEQUAL OP_IF
RECOVERY OP_CHECKSIGVERIFY cd50 OP_CHECKSEQUENCEVERIFY OP_0NOTEQUAL
OP_ENDIF
OP_ADD 2 OP_EQUAL
```

The members key, which is MPC generated by Mixin Safe nodes, is controlled in a decentralized manner. Whenever a deposit is made into a safe account, Mixin Safe issues an equivalent amount of safeBTC to the account owner. To initiate a transaction with the owner key, the user needs to send safeBTC to the Mixin Safe network and sign the raw transaction with the owner key, thereby enabling the members to sign the transaction together with the owner key.

![Mixin Safe](https://raw.githubusercontent.com/MixinNetwork/safe/main/observer/assets/safe-flow.png)

## Prepare Owner Key

To better understand the concept, it's recommended to write some code. We will use btcd, which can be accessed at https://github.com/btcsuite/btcd.

Using btcd, you can generate a public and private key pair using the following code:

```golang
priv, pub := btcec.PrivKeyFromBytes(seed)
fmt.Printf("public: %x\nprivate: %x\n", pub.SerializeCompressed(), priv.Serialize())

🔜
public: 039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40
private: 1b639e995830c253eb38780480440a72919f5448be345a574c545329f2df4d76
```

After generating the key pair, you will need to create a random UUID as the session ID. As example, the UUID `2e78d04a-e61a-442d-a014-dec19bd61cfe` will be used.


## Propose Safe Account

To ensure the efficiency of the network, every Mixin Safe account proposal costs 1USD. To propose an account, one simply needs to send 20pUSD to the network with a properly encoded memo. All messages sent to the safe network must be encoded as per the following operation structure:

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

To send the account proposal, with the owner key prepared from last step, the operation value should be like:

```golang
op := &Operation {
  Id: "2e78d04a-e61a-442d-a014-dec19bd61cfe",
  Type: 110,
  Curve: 1,
  Public: "039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40",
}
```

All above four fields above are mandatory for all safe network transactions, now to propose a safe account with 7 days timelock, we need to make the extra:

```golang
timelock := binary.BigEndian.AppendUint16(nil, 24*7)
threshold := byte(1)
total := byte(1)
owners := []string{"fcb87491-4fa0-4c2f-b387-262b63cbc112"}
extra := append(timelock, threshold, total)
uid := uuid.FromStringOrNil(owners[0])
op.Extra = append(extra, uid.Bytes()...)
```

So the safe account proposal operation extra is encoded with timelock in hours, threshold, owners count, and all owner UUIDs.

Then we can encode the operation and use it as a memo to send the account proposal transaction to safe network MTG:

```golang
memo := base64.RawURLEncoding.EncodeToString(op.Encode())
input := mixin.TransferInput{
  AssetID: "31d2ea9c-95eb-3355-b65b-ba096853bc18",
  Amount:  decimal.NewFromFloat(1),
  TraceID: op.Id,
  Memo:    memo,
}
input.OpponentMultisig.Receivers = []{
  "a4930d3e-4783-4ccd-ae3e-f6651b5583c7",
  "2cf5645b-5c52-42e4-8c67-ed5164cfe8eb",
  "335654a7-986d-4600-ab89-b624e9998f36",
  "3d963e3c-2dd3-4902-b340-e8394d62ad0f",
  "ed3d5824-87e4-4060-b347-90b3a3aa16fb",
  "a8327607-724d-45d4-afca-339d33219d1a",
  "9ad6076e-c79d-4571-b29a-4671262c2538",
  "b1081493-d702-43e1-8051-cec283e9898f",
  "f5a9bf39-2e3d-49d9-bbfc-144aaf209157",
  "bfe8c7b9-58a3-4d2d-92b4-ba5b67eb1a42",
  "da9bdc94-a446-422c-ab90-8ab9c5bb8bc7",
  "9fcdea14-03d1-49f1-af97-4079c9551777",
  "8cf9b500-0bc8-408e-890b-41873e162345",
  "72b336e4-1e05-477a-8254-2f02a6249ffd",
  "5ae7f5cf-26b8-4ea6-b031-2bf3af09da57",
  "18f2c8ad-ac9b-4a6f-a074-240bfacbe58b",
  "21da6e56-f335-45c4-a838-9a0139fe7269",
  "83170828-5bd8-491d-9bb0-f1af072c305b",
  "40032eda-126b-44f2-bfb9-76da965cf0c2",
  "fb264547-198d-4877-9ef9-66f6b3f4e3d7",
  "a3a68c12-2407-4c3b-ad5d-5c37a3d29b1a",
  "77a3a6fe-fc4c-4035-8409-0f4b5daba51d",
  "1e3c4323-207d-4d7b-bcd6-21b35d02bdb7",
  "fca01bd7-3e87-4d9e-bf88-cbd8f642cc16",
  "7552beb9-4a7b-4cbb-a026-f4db1d86cbf9",
  "575ede5a-4802-42e8-81b1-6b2e2ef187d8",
  "07775ff6-bb41-4fbd-9f81-8e600898ee6e",
  "c91eb626-eb89-4fbd-ae21-76f0bd763da5",
}
input.OpponentMultisig.Threshold = 19
```


## Approve Safe Account

After the account proposal transaction sent to the safe network MTG, you can monitor the Mixin Network transactions to decode your account details. But to make it easy, it's possible to just fetch it from the Safe HTTP API with the proposal UUID:

```
curl https://observer.mixin.one/accounts/2e78d04a-e61a-442d-a014-dec19bd61cfe

🔜
{
  "address":"bc1qzccxhrlm4p5l5rpgnns58862ckmsat7uxucqjfcfmg7ef6yltf3quhr94a",
  "id":"2e78d04a-e61a-442d-a014-dec19bd61cfe",
  "script":"6352670399f...96e9e0bc052ae",
  "state":"proposed"
}
```

You should have noticed that the request was made with the same session UUID we prepared at the first step. That address returned is our safe account address to receive BTC, but before using it, we must approve it with our owner key:


```golang
var buf bytes.Buffer
_ = wire.WriteVarString(&buf, 0, "Bitcoin Signed Message:\n")
_ = wire.WriteVarString(&buf, 0, fmt.Sprintf("APPROVE:%s:%s", sessionUUID, address))
hash := chainhash.DoubleHashB(buf.Bytes())
b, _ := hex.DecodeString(priv)
private, _ := btcec.PrivKeyFromBytes(b)
sig := ecdsa.Sign(private, hash)
fmt.Println(base64.RawURLEncoding.EncodeToString(sig.Serialize()))

🔜
MEUCIQCY3Gl1uocJR-qa2wVUuvK_gc-pOxzk8Zq_x_Hqv8iJbAIgXPbMuk-GiGsM3MJKmQ3haRzfDEKSBHArkgRF2NtxDOk
```

With the signature we send the request to safe network to prove that we own the owner key exactly:

```
curl https://observer.mixin.one/accounts/2e78d04a-e61a-442d-a014-dec19bd61cfe -H 'Content-Type:application/json' \
  -d '{"address":"bc1qzccxhrlm4p5l5rpgnns58862ckmsat7uxucqjfcfmg7ef6yltf3quhr94a","signature":"MEUCIQCY3Gl1uocJR-qa2wVUuvK_gc-pOxzk8Zq_x_Hqv8iJbAIgXPbMuk-GiGsM3MJKmQ3haRzfDEKSBHArkgRF2NtxDOk"}'
```

Now we can deposit BTC to the address above, and you will receive safeBTC to the owner wallet.


## Propose Safe Transaction

After depositing some BTC to both the safe address, we now want to send 0.000123 BTC to `bc1qevu9qqpfqp4s9jq3xxulfh08rgyjy8rn76aj7e`. To initiate the transaction, we require the latest Bitcoin chain head ID from the Safe network, which can be obtained by running the following command:

```
curl https://observer.mixin.one/chains

🔜
[
  {
    "chain": 1,
    "head": {
      "fee": 13,
      "hash": "00000000000000000003aca37e964e47e89543e2b26495641c1fc4957500e46e",
      "height": 780626,
      "id": "155e4f85-d4b8-33f7-82e6-542711f1f26e"
    },
    "id": "c6d0c728-2624-429b-8e0d-d9d19b6592fa"
  }
]
```

Using the response we receive, we can determine that the Bitcoin transaction fee rate will be `13 Satoshis/vByte`. We will then include the head ID `155e4f85-d4b8-33f7-82e6-542711f1f26e` in the operation extra to indicate the fee rate we prefer.

Furthermore, we need to generate another random session ID, for which we will use `36c2075c-5af0-4593-b156-e72f58f9f421` as an example. With the owner key prepared in the first step, the operation value should be as follows:

```golang
extra := []byte{0} // Start with 0 to propose a normal transaction
extra = append(extra, uuid.FromStringOrNil("155e4f85-d4b8-33f7-82e6-542711f1f26e").Bytes()...)
extra = append(extra, []byte("bc1qevu9qqpfqp4s9jq3xxulfh08rgyjy8rn76aj7e")...)
op := &Operation {
  Id: "36c2075c-5af0-4593-b156-e72f58f9f421",
  Type: 112,
  Curve: 1,
  Public: "039c2f5ebdd4eae6d69e7a98b737beeb78e0a8d42c7b957a0fbe0c41658d16ab40",
  Extra: extra,
}
```

Next, we need to retrieve the safeBTC asset ID that was provided to us when we deposited BTC to the safe address. It is important to note that each safe account has its own unique safe asset ID, and for the safe account we created, the safeBTC asset ID is `94683442-3ae2-3118-bec7-069c934668c0`. We will use this asset ID to make a transaction to the Safe Network MTG as follows:

```golang
memo := base64.RawURLEncoding.EncodeToString(op.Encode())
input := mixin.TransferInput{
  AssetID: "94683442-3ae2-3118-bec7-069c934668c0",
  Amount:  decimal.NewFromFloat(0.000123),
  TraceID: op.Id,
  Memo:    memo,
}
input.OpponentMultisig.Receivers = []{
  "a4930d3e-4783-4ccd-ae3e-f6651b5583c7",
  "2cf5645b-5c52-42e4-8c67-ed5164cfe8eb",
  "335654a7-986d-4600-ab89-b624e9998f36",
  "3d963e3c-2dd3-4902-b340-e8394d62ad0f",
  "ed3d5824-87e4-4060-b347-90b3a3aa16fb",
  "a8327607-724d-45d4-afca-339d33219d1a",
  "9ad6076e-c79d-4571-b29a-4671262c2538",
  "b1081493-d702-43e1-8051-cec283e9898f",
  "f5a9bf39-2e3d-49d9-bbfc-144aaf209157",
  "bfe8c7b9-58a3-4d2d-92b4-ba5b67eb1a42",
  "da9bdc94-a446-422c-ab90-8ab9c5bb8bc7",
  "9fcdea14-03d1-49f1-af97-4079c9551777",
  "8cf9b500-0bc8-408e-890b-41873e162345",
  "72b336e4-1e05-477a-8254-2f02a6249ffd",
  "5ae7f5cf-26b8-4ea6-b031-2bf3af09da57",
  "18f2c8ad-ac9b-4a6f-a074-240bfacbe58b",
  "21da6e56-f335-45c4-a838-9a0139fe7269",
  "83170828-5bd8-491d-9bb0-f1af072c305b",
  "40032eda-126b-44f2-bfb9-76da965cf0c2",
  "fb264547-198d-4877-9ef9-66f6b3f4e3d7",
  "a3a68c12-2407-4c3b-ad5d-5c37a3d29b1a",
  "77a3a6fe-fc4c-4035-8409-0f4b5daba51d",
  "1e3c4323-207d-4d7b-bcd6-21b35d02bdb7",
  "fca01bd7-3e87-4d9e-bf88-cbd8f642cc16",
  "7552beb9-4a7b-4cbb-a026-f4db1d86cbf9",
  "575ede5a-4802-42e8-81b1-6b2e2ef187d8",
  "07775ff6-bb41-4fbd-9f81-8e600898ee6e",
  "c91eb626-eb89-4fbd-ae21-76f0bd763da5",
}
input.OpponentMultisig.Threshold = 19
```

Once the transaction is successfully sent to the Safe Network MTG, we can query the safe API to obtain the proposed raw transaction using the following command:

```
curl https://observer.mixin.one/transactions/36c2075c-5af0-4593-b156-e72f58f9f421

🔜
{
  "chain":1,
  "hash":"0e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f",
  "id":"36c2075c-5af0-4593-b156-e72f58f9f421",
  "raw":"00200e88c368c51fb...000000000000000007db5"
}
```


## Approve Safe Transaction

With the transaction proposed in previous step, we can decode the raw response to get the partially signed bitcoin transaction for the owner key to sign. Then with the decoded PSBT, we can parse it to see all its inputs and outputs, and verify it's correct as we proposed. Then we sign all signature hashes with our owner private key:

```golang
script := theSafeAccountScript()
psbtBytes, _ := hex.DecodeString(raw)
pkt, _ = psbt.NewFromRawBytes(bytes.NewReader(psbtBytes), false)
for idx := range pkt.UnsignedTx.TxIn {
	hash := sigHash(pkt, idx)
	sig := ecdsa.Sign(owner, hash).Serialize()
	pkt.Inputs[idx].PartialSigs = []*psbt.PartialSig{{
		PubKey:    owner.PubKey().SerializeCompressed(),
		Signature: sig,
	}}
}
raw := marshal(hash, pkt, fee)
fmt.Printf("raw: %x\n", raw)
```

After we have the PSBT signed by owner private key, then we can send them to safe API:

```
curl https://observer.mixin.one/transactions/36c2075c-5af0-4593-b156-e72f58f9f421 -H 'Content-Type:application/json' \
  -d '{"action":"approve","chain":1,"raw":"00200e88c368c51fb...000000000000000007db5"}'
```

Once the transaction approval has succeeded, we will need to transfer 20pUSD to Mixin Safe Observer node(c91eb626-eb89-4fbd-ae21-76f0bd763da5), using the transaction hash as the memo to pay for it. After a few minutes, we should be able to query the transaction on a Bitcoin explorer and view its details.

https://blockstream.info/tx/0e88c368c51fb24421b2a36d82674a5f058eb98d67da844d393b8df00ad2ad3f?expand


## Custom Recovery Key

It's possible to have your own recovery key instead of using the managed recovery service provided by Mixin Safe. At first you need to prepare your recovery public key and a chain code according to Bitcoin extended public key specification. Then add this key to Mixin Safe Observer node(c91eb626-eb89-4fbd-ae21-76f0bd763da5) by transferring 100pUSD, and the memo should be:

```golang
const CurveSecp256k1ECDSABitcoin = 1
extra := []byte{CurveSecp256k1ECDSABitcoin}
extra = append(extra, public...)
extra = append(extra, chainCode...)
memo := base64.RawURLEncoding.EncodeToString(extra)
```

After your own recovery key successfully added to Safe Network, you can start proposing a safe account as before, the only modification is appending the recovery public key bytes to the operation extra.
