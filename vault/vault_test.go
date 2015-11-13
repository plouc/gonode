package vault

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

var largeMessage []byte
var smallMessage []byte
var xLargeMessage []byte

var key = []byte("de4d3ae8cf578c971b39ab5f21b2435483a3654f63b9f3777925c77e9492a141")

func init() {
	smallMessage = []byte("Comment ca va ??")

	largeMessage = make([]byte, 1024*1024*1+2)
	io.ReadFull(rand.Reader, largeMessage)

	fmt.Println("Start generating XLarge message")
	xLargeMessage = make([]byte, 1024*1024*10+3)
	io.ReadFull(rand.Reader, xLargeMessage)
	fmt.Println("End generating XLarge message")
}

// write/encrypted file
func RunTestVault(t *testing.T, v *Vault, plaintext []byte, msgPrefix string) {
	var read int64

	file := "this-is-a-test"

	meta := NewVaultMetadata()
	meta["foo"] = "bar"

	reader := bytes.NewBuffer(plaintext)

	written, err := v.Put(file, meta, reader)

	assert.NoError(t, err, msgPrefix+"err returned")
	assert.True(t, written >= int64(len(plaintext)), msgPrefix) // some cipher might add extra data
	assert.True(t, written > 0, msgPrefix)                      // some cipher might add extra data
	assert.True(t, v.Has(file), msgPrefix)

	invalid := []byte("Another invalid message with the same key")

	// test overwrite
	written, err = v.Put(file, meta, bytes.NewBuffer(invalid))
	assert.Error(t, err, msgPrefix)
	assert.Equal(t, written, int64(0), msgPrefix)

	// get metadata
	meta, err = v.GetMeta(file)
	assert.NoError(t, err, msgPrefix)
	assert.Equal(t, meta["foo"].(string), "bar", msgPrefix)

	// get file
	writer := bytes.NewBuffer([]byte(""))
	read, err = v.Get(file, writer)
	assert.Equal(t, read, int64(len(plaintext)), msgPrefix)
	assert.True(t, len(plaintext) > 0, "plaintext length should not be empty", msgPrefix)
	assert.NoError(t, err, msgPrefix)
	assert.Equal(t, plaintext, writer.Bytes(), msgPrefix)

	// remove file
	v.Remove(file)
	assert.NoError(t, err, msgPrefix)
}

// read stored encrypted files
func RunRegressionTest(t *testing.T, v *Vault) {
	file := "The secret file"

	assert.True(t, v.Has(file))

	meta, err := v.GetMeta(file)
	assert.NoError(t, err)

	assert.Equal(t, meta["foo"].(string), "bar")

	writer := bytes.NewBufferString("")
	_, err = v.Get(file, writer)
	assert.NoError(t, err)
	assert.Equal(t, writer.String(), "The secret message")
}

func Test_VaultElement(t *testing.T) {
	ve := NewVaultElement()

	assert.Equal(t, ve.Algo, "aes_ctr") // default value
	assert.NotEmpty(t, ve.BinKey)
	assert.NotEmpty(t, ve.MetaKey)
}

func Test_AesEncrypt(t *testing.T) {
	ve := NewVaultElement()
	ve.MetaKey = []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	assert.Equal(t, 32, len(ve.MetaKey))

	src := bytes.NewBuffer([]byte("Hello World!!"))
	dst := bytes.NewBuffer([]byte(""))

	AesOFBEncrypter(ve.MetaKey, src, dst)

	assert.NotEmpty(t, dst.String())

	decrypted := bytes.NewBuffer([]byte(""))

	AesOFBDecrypter(ve.MetaKey, dst, decrypted)

	assert.Equal(t, []byte("Hello World!!"), decrypted.Bytes())
}
