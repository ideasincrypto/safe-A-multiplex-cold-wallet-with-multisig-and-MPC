package abi

import (
	"context"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/MixinNetwork/mixin/logger"
	"github.com/MixinNetwork/safe/apps/ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gofrs/uuid/v5"
)

var factoryContractAddress string

func InitFactoryContractAddress(addr string) {
	addr = strings.ToLower(addr)
	if ethereum.VerifyAssetKey(addr) != nil {
		panic(addr)
	}
	if factoryContractAddress == "" {
		factoryContractAddress = addr
	}
	if factoryContractAddress != addr {
		panic(factoryContractAddress)
	}
}

func GetOrDeployFactoryAsset(ctx context.Context, rpc, key string, assetId, symbol, name, receiver, holder string) error {
	conn, abi, err := factoryInit(rpc)
	if err != nil {
		return err
	}
	defer conn.Close()

	addr := GetFactoryAssetAddress(receiver, assetId, symbol, name, holder)
	deployed, err := CheckFactoryAssetDeployed(rpc, addr.String())
	logger.Printf("abi.CheckFactoryAssetDeployed(%s, %s) => %v %v", rpc, addr, deployed, err)
	if err != nil || deployed.Sign() > 0 {
		return err
	}

	signer, err := signerInit(key)
	if err != nil {
		return err
	}
	id := new(big.Int).SetBytes(uuid.Must(uuid.FromString(assetId)).Bytes())
	symbol, name = "safe"+symbol, name+" @ Mixin Safe"
	t, err := abi.Deploy(signer, common.HexToAddress(receiver), id, holder, symbol, name)
	if err != nil {
		return err
	}
	rb, err := t.MarshalBinary()
	if err != nil {
		panic(err)
	}
	logger.Printf("abi.Deploy(%s, %s, %s, %s, %s) => %s %x %d %d %s %s", receiver,
		id, holder, symbol, name, t.Hash().Hex(), rb, t.Nonce(), t.Gas(), t.GasFeeCap(), t.GasTipCap())
	_, err = bind.WaitMined(ctx, conn, t)
	logger.Printf("abi.WaitMined(%v) => %v", t, err)
	return err
}

func CheckFactoryAssetDeployed(rpc, assetKey string) (*big.Int, error) {
	conn, abi, err := factoryInit(rpc)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	addr := common.HexToAddress(assetKey)
	return abi.Assets(nil, addr)
}

func GetFactoryAssetAddress(receiver, assetId, symbol, name string, holder string) common.Address {
	symbol, name = "safe"+symbol, name+" @ Mixin Safe"
	id := uuid.Must(uuid.FromString(assetId))
	args := common.HexToAddress(receiver).Bytes()
	args = append(args, math.U256Bytes(new(big.Int).SetBytes(id.Bytes()))...)
	args = append(args, holder...)
	args = append(args, symbol...)
	args = append(args, name...)
	salt := crypto.Keccak256(args)

	code, err := hex.DecodeString(polygonAssetContractCode[2:])
	if err != nil {
		panic(err)
	}
	code = append(code, PackAssetArguments(symbol, name)...)
	this, err := hex.DecodeString(factoryContractAddress[2:])
	if err != nil {
		panic(err)
	}

	input := []byte{0xff}
	input = append(input, this...)
	input = append(input, math.U256Bytes(new(big.Int).SetBytes(salt))...)
	input = append(input, crypto.Keccak256(code)...)
	return common.BytesToAddress(crypto.Keccak256(input))
}

func factoryInit(rpc string) (*ethclient.Client, *FactoryContract, error) {
	conn, err := ethclient.Dial(rpc)
	if err != nil {
		return nil, nil, err
	}

	abi, err := NewFactoryContract(common.HexToAddress(factoryContractAddress), conn)
	if err != nil {
		return nil, nil, err
	}

	return conn, abi, nil
}

func signerInit(key string) (*bind.TransactOpts, error) {
	chainId := new(big.Int).SetInt64(ethereumChainId)
	priv, err := crypto.HexToECDSA(key)
	if err != nil {
		return nil, err
	}
	return bind.NewKeyedTransactorWithChainID(priv, chainId)
}

func PackAssetArguments(symbol, name string) []byte {
	stringTy, err := abi.NewType("string", "", nil)
	if err != nil {
		panic(err)
	}

	arguments := abi.Arguments{
		{
			Type: stringTy,
		},
		{
			Type: stringTy,
		},
	}

	args, err := arguments.Pack(
		symbol,
		name,
	)
	if err != nil {
		panic(err)
	}
	return args
}

const (
	polygonAssetContractCode = "0x608060405234801561000F575F80FD5B5060405161120C38038061120C83398181016040528101906100319190610206565B81600390816100409190610489565B5080600290816100509190610489565B507FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5F803373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20819055505050610558565B5F604051905090565B5F80FD5B5F80FD5B5F80FD5B5F80FD5B5F601F19601F8301169050919050565B7F4E487B71000000000000000000000000000000000000000000000000000000005F52604160045260245FFD5B610118826100D2565B810181811067FFFFFFFFFFFFFFFF82111715610137576101366100E2565B5B80604052505050565B5F6101496100B9565B9050610155828261010F565B919050565B5F67FFFFFFFFFFFFFFFF821115610174576101736100E2565B5B61017D826100D2565B9050602081019050919050565B8281835E5F83830152505050565B5F6101AA6101A58461015A565B610140565B9050828152602081018484840111156101C6576101C56100CE565B5B6101D184828561018A565B509392505050565B5F82601F8301126101ED576101EC6100CA565B5B81516101FD848260208601610198565B91505092915050565B5F806040838503121561021C5761021B6100C2565B5B5F83015167FFFFFFFFFFFFFFFF811115610239576102386100C6565B5B610245858286016101D9565B925050602083015167FFFFFFFFFFFFFFFF811115610266576102656100C6565B5B610272858286016101D9565B9150509250929050565B5F81519050919050565B7F4E487B71000000000000000000000000000000000000000000000000000000005F52602260045260245FFD5B5F60028204905060018216806102CA57607F821691505B6020821081036102DD576102DC610286565B5B50919050565B5F819050815F5260205F209050919050565B5F6020601F8301049050919050565B5F82821B905092915050565B5F6008830261033F7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF82610304565B6103498683610304565B95508019841693508086168417925050509392505050565B5F819050919050565B5F819050919050565B5F61038D61038861038384610361565B61036A565B610361565B9050919050565B5F819050919050565B6103A683610373565B6103BA6103B282610394565B848454610310565B825550505050565B5F90565B6103CE6103C2565B6103D981848461039D565B505050565B5B818110156103FC576103F15F826103C6565B6001810190506103DF565B5050565B601F82111561044157610412816102E3565B61041B846102F5565B8101602085101561042A578190505B61043E610436856102F5565B8301826103DE565B50505B505050565B5F82821C905092915050565B5F6104615F1984600802610446565B1980831691505092915050565B5F6104798383610452565B9150826002028217905092915050565B6104928261027C565B67FFFFFFFFFFFFFFFF8111156104AB576104AA6100E2565B5B6104B582546102B3565B6104C0828285610400565B5F60209050601F8311600181146104F1575F84156104DF578287015190505B6104E9858261046E565B865550610550565B601F1984166104FF866102E3565B5F5B8281101561052657848901518255600182019150602085019450602081019050610501565B86831015610543578489015161053F601F891682610452565B8355505B6001600288020188555050505B505050505050565B610CA7806105655F395FF3FE608060405234801561000F575F80FD5B5060043610610091575F3560E01C8063313CE56711610064578063313CE5671461013157806370A082311461014F57806395D89B411461017F578063A9059CBB1461019D578063DD62ED3E146101CD57610091565B806306FDDE0314610095578063095EA7B3146100B357806318160DDD146100E357806323B872DD14610101575B5F80FD5B61009D6101FD565B6040516100AA91906108E2565B60405180910390F35B6100CD60048036038101906100C89190610993565B610289565B6040516100DA91906109EB565B60405180910390F35B6100EB61043A565B6040516100F89190610A13565B60405180910390F35B61011B60048036038101906101169190610A2C565B61045E565B60405161012891906109EB565B60405180910390F35B610139610475565B6040516101469190610A97565B60405180910390F35B61016960048036038101906101649190610AB0565B61047A565B6040516101769190610A13565B60405180910390F35B6101876104BF565B60405161019491906108E2565B60405180910390F35B6101B760048036038101906101B29190610993565B61054B565B6040516101C491906109EB565B60405180910390F35B6101E760048036038101906101E29190610ADB565B610561565B6040516101F49190610A13565B60405180910390F35B6002805461020A90610B46565B80601F016020809104026020016040519081016040528092919081815260200182805461023690610B46565B80156102815780601F1061025857610100808354040283529160200191610281565B820191905F5260205F20905B81548152906001019060200180831161026457829003601F168201915B505050505081565B5F8082148061030F57505F60015F3373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F205F8573FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F2054145B61034E576040517F08C379A000000000000000000000000000000000000000000000000000000000815260040161034590610BC0565B60405180910390FD5B8160015F3373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F205F8573FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20819055508273FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF163373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF167F8C5BE1E5EBEC7D5BD14F71427D1E84F3DD0314C0F7B2291E5B200AC8C7C3B925846040516104289190610A13565B60405180910390A36001905092915050565B7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF81565B5F61046A8484846105E3565B600190509392505050565B601281565B5F805F8373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20549050919050565B600380546104CC90610B46565B80601F01602080910402602001604051908101604052809291908181526020018280546104F890610B46565B80156105435780601F1061051A57610100808354040283529160200191610543565B820191905F5260205F20905B81548152906001019060200180831161052657829003601F168201915B505050505081565B5F6105573384846106F8565B6001905092915050565B5F60015F8473FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F205F8373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F2054905092915050565B5F60015F8573FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F205F3373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20549050818161066B9190610C0B565B60015F8673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F205F3373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20819055506106F28484846106F8565B50505050565B805F808573FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20546107409190610C0B565B5F808573FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F2081905550805F808473FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20546107C89190610C3E565B5F808473FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1673FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF1681526020019081526020015F20819055508173FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF168373FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF167FDDF252AD1BE2C89B69C2B068FC378DAA952BA7F163C4A11628F55A4DF523B3EF836040516108659190610A13565B60405180910390A3505050565B5F81519050919050565B5F82825260208201905092915050565B8281835E5F83830152505050565B5F601F19601F8301169050919050565B5F6108B482610872565B6108BE818561087C565B93506108CE81856020860161088C565B6108D78161089A565B840191505092915050565B5F6020820190508181035F8301526108FA81846108AA565B905092915050565B5F80FD5B5F73FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF82169050919050565B5F61092F82610906565B9050919050565B61093F81610925565B8114610949575F80FD5B50565B5F8135905061095A81610936565B92915050565B5F819050919050565B61097281610960565B811461097C575F80FD5B50565B5F8135905061098D81610969565B92915050565B5F80604083850312156109A9576109A8610902565B5B5F6109B68582860161094C565B92505060206109C78582860161097F565B9150509250929050565B5F8115159050919050565B6109E5816109D1565B82525050565B5F6020820190506109FE5F8301846109DC565B92915050565B610A0D81610960565B82525050565B5F602082019050610A265F830184610A04565B92915050565B5F805F60608486031215610A4357610A42610902565B5B5F610A508682870161094C565B9350506020610A618682870161094C565B9250506040610A728682870161097F565B9150509250925092565B5F60FF82169050919050565B610A9181610A7C565B82525050565B5F602082019050610AAA5F830184610A88565B92915050565B5F60208284031215610AC557610AC4610902565B5B5F610AD28482850161094C565B91505092915050565B5F8060408385031215610AF157610AF0610902565B5B5F610AFE8582860161094C565B9250506020610B0F8582860161094C565B9150509250929050565B7F4E487B71000000000000000000000000000000000000000000000000000000005F52602260045260245FFD5B5F6002820490506001821680610B5D57607F821691505B602082108103610B7057610B6F610B19565B5B50919050565B7F617070726F7665206F6E2061206E6F6E2D7A65726F20616C6C6F77616E6365005F82015250565B5F610BAA601F8361087C565B9150610BB582610B76565B602082019050919050565B5F6020820190508181035F830152610BD781610B9E565B9050919050565B7F4E487B71000000000000000000000000000000000000000000000000000000005F52601160045260245FFD5B5F610C1582610960565B9150610C2083610960565B9250828203905081811115610C3857610C37610BDE565B5B92915050565B5F610C4882610960565B9150610C5383610960565B9250828201905080821115610C6B57610C6A610BDE565B5B9291505056FEA2646970667358221220D0ABD1B55CF88967F363DB9F2246BA6E5FCD548709BD169ECCD0B7774AC9AD9064736F6C63430008190033"
	ethereumChainId          = 137
)
