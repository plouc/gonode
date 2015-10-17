package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/elgs/gostrgen"
	"io"
)

var VaultFileExistsError = errors.New("Vault file already exists")

type VaultMetadata map[string]interface{}

func NewVaultMetadata() VaultMetadata {
	return make(VaultMetadata)
}

type Vault interface {
	Has(key string) bool
	Get(key string) (VaultMetadata, error)
	GetReader(key string) (io.Reader, error)
	Put(key string, meta VaultMetadata, r io.Reader) (int64, error)
	Remove(key string) error
}

type Encrypter func(key interface{}, w io.Writer) io.Writer
type Decrypter func(key interface{}, r io.Reader) io.Reader

type VaultElement struct {
	Key  []byte `json:"key"`
	Algo string `json:"algo"`
	Hash string `json:"hash"`
}

func GetVaultKey(name string) []byte {
	sum := sha256.Sum256([]byte(name))

	return sum[:]
}

func GetVaultPath(sum []byte) string {
	return fmt.Sprintf("%x/%x/%x.bin", sum[0:1], sum[1:2], sum[2:])
}

func generateKey() []byte {
	str, _ := gostrgen.RandGen(32, gostrgen.All, "", "")

	return []byte(str)
}

func NewVaultElement() *VaultElement {
	return &VaultElement{
		Algo: "aes",
		Key:  generateKey(),
	}
}

func NoopEncrypter(key interface{}, w io.Writer) io.Writer {
	return w
}

func NoopDecrypter(key interface{}, r io.Reader) io.Reader {
	return r
}

func AesOFBEncrypter(key interface{}, w io.Writer) io.Writer {
	block, err := aes.NewCipher(key.([]byte))
	if err != nil {
		panic(err)
	}

	// If the key is unique for each ciphertext, then it's ok to use a zero
	// IV.
	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(block, iv[:])

	return &cipher.StreamWriter{S: stream, W: w}
}

func AesOFBDecrypter(key interface{}, r io.Reader) io.Reader {
	block, err := aes.NewCipher(key.([]byte))
	if err != nil {
		panic(err)
	}

	// If the key is unique for each ciphertext, then it's ok to use a zero
	// IV.
	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(block, iv[:])

	reader := &cipher.StreamReader{S: stream, R: r}

	return reader
}