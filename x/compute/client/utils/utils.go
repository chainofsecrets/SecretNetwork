package utils

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/decred/dcrd/dcrec/secp256k1"
	regtypes "github.com/enigmampc/EnigmaBlockchain/x/registration"
	ra "github.com/enigmampc/EnigmaBlockchain/x/registration/remote_attestation"
	"github.com/miscreant/miscreant.go"
	"golang.org/x/crypto/hkdf"
)

var (
	gzipIdent = []byte("\x1F\x8B\x08")
	wasmIdent = []byte("\x00\x61\x73\x6D")
)

// IsGzip returns checks if the file contents are gzip compressed
func IsGzip(input []byte) bool {
	return bytes.Equal(input[:3], gzipIdent)
}

// IsWasm checks if the file contents are of wasm binary
func IsWasm(input []byte) bool {
	return bytes.Equal(input[:4], wasmIdent)
}

// GzipIt compresses the input ([]byte)
func GzipIt(input []byte) ([]byte, error) {
	// Create gzip writer.
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, err := w.Write(input)
	if err != nil {
		return nil, err
	}
	err = w.Close() // You must close this first to flush the bytes to the buffer.
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// WASMCLIContext wraps github.com/cosmos/cosmos-sdk/client/context.CLIContext
type WASMCLIContext struct {
	CLIContext context.CLIContext
}

type keyPair struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}

func (ctx WASMCLIContext) getTxSenderKeyPair() (*secp256k1.PrivateKey, *secp256k1.PublicKey, error) {
	keyPairFilePath := path.Join(ctx.CLIContext.HomeDir, "id_tx_io.json")

	if _, err := os.Stat(keyPairFilePath); os.IsNotExist(err) {
		privkey, err := secp256k1.GeneratePrivateKey()
		if err != nil {
			return nil, nil, err
		}

		pubkey := privkey.PubKey()

		keyPair := keyPair{
			Private: hex.EncodeToString(privkey.Serialize()),
			Public:  hex.EncodeToString(pubkey.SerializeCompressed()),
		}

		keyPairJSONBytes, err := json.MarshalIndent(keyPair, "", "    ")
		if err != nil {
			return nil, nil, err
		}

		err = ioutil.WriteFile(keyPairFilePath, keyPairJSONBytes, 0644)
		if err != nil {
			return nil, nil, err
		}

		return privkey, pubkey, nil
	}

	keyPairJSONBytes, err := ioutil.ReadFile(keyPairFilePath)
	if err != nil {
		return nil, nil, err
	}

	var keyPair keyPair

	err = json.Unmarshal(keyPairJSONBytes, &keyPair)
	if err != nil {
		return nil, nil, err
	}

	privKeyBytes, err := hex.DecodeString(keyPair.Private)

	privkey, pubkey := secp256k1.PrivKeyFromBytes(privKeyBytes)
	return privkey, pubkey, nil
}

var hkdfSalt = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x4b, 0xea, 0xd8, 0xdf, 0x69, 0x99,
	0x08, 0x52, 0xc2, 0x02, 0xdb, 0x0e, 0x00, 0x97, 0xc1, 0xa1, 0x2e, 0xa6, 0x37, 0xd7, 0xe9, 0x6d}

func (ctx WASMCLIContext) getTxEncryptionKey(txSenderPrivKey *secp256k1.PrivateKey, nonce []byte) ([]byte, error) {
	res, _, err := ctx.CLIContext.Query("custom/register/master-cert")
	if err != nil {
		return nil, err
	}
	var certs regtypes.GenesisState

	err = json.Unmarshal(res, &certs)
	if err != nil {
		return nil, err
	}

	ioPubKeyBytes, err := ra.VerifyRaCert(certs.IoMasterCertificate)
	if err != nil {
		return nil, err
	}
	ioPubKey, err := secp256k1.ParsePubKey(ioPubKeyBytes)
	if err != nil {
		return nil, err
	}

	txEncryptionIkm := secp256k1.GenerateSharedSecret(txSenderPrivKey, ioPubKey)

	hkdf := hkdf.New(sha256.New, append(txEncryptionIkm, nonce...), hkdfSalt, []byte{})

	txEncryptionKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, txEncryptionKey); err != nil {
		return nil, err
	}

	return txEncryptionKey, nil
}

// Encrypt encrypts
func (ctx WASMCLIContext) Encrypt(plaintext []byte) ([]byte, error) {
	txSenderPrivKey, txSenderPubKey, err := ctx.getTxSenderKeyPair()

	nonce := make([]byte, 32)
	rand.Read(nonce)

	txEncryptionKey, err := ctx.getTxEncryptionKey(txSenderPrivKey, nonce)
	if err != nil {
		return nil, err
	}

	cipher, err := miscreant.NewAESCMACSIV(txEncryptionKey)
	if err != nil {
		return nil, err
	}

	ciphertext, err := cipher.Seal(nil, plaintext, []byte{})
	if err != nil {
		return nil, err
	}

	// ciphertext = nonce(32) || wallet_pubkey(33) || ciphertext
	ciphertext = append(nonce, append(txSenderPubKey.SerializeCompressed(), ciphertext...)...)

	return ciphertext, nil
}

// Decrypt decrypts
func (ctx WASMCLIContext) Decrypt(ciphertext []byte, nonce []byte) ([]byte, error) {
	txSenderPrivKey, _, err := ctx.getTxSenderKeyPair()

	txEncryptionKey, err := ctx.getTxEncryptionKey(txSenderPrivKey, nonce)
	if err != nil {
		return nil, err
	}

	cipher, err := miscreant.NewAESCMACSIV(txEncryptionKey)
	if err != nil {
		return nil, err
	}

	return cipher.Open(nil, ciphertext, []byte{})
}
