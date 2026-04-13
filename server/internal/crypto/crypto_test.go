package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef") // 32 bytes
}

func TestEncryptDecrypt(t *testing.T) {
	t.Parallel()

	key := testKey()
	plain := "my-secret-password"

	encrypted, err := Encrypt(key, plain)
	require.NoError(t, err)
	assert.True(t, IsEncrypted(encrypted))

	decrypted, err := Decrypt(key, encrypted)
	require.NoError(t, err)
	assert.Equal(t, plain, decrypted)
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	t.Parallel()

	key := testKey()

	encrypted, err := Encrypt(key, "")
	require.NoError(t, err)
	assert.True(t, IsEncrypted(encrypted))

	decrypted, err := Decrypt(key, encrypted)
	require.NoError(t, err)
	assert.Equal(t, "", decrypted)
}

func TestDecrypt_PlaintextPassthrough(t *testing.T) {
	t.Parallel()

	key := testKey()

	// A value without the "enc:" prefix is treated as legacy plaintext.
	plain, err := Decrypt(key, "plain-password")
	require.NoError(t, err)
	assert.Equal(t, "plain-password", plain)
}

func TestEncrypt_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	_, err := Encrypt([]byte("short"), "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestDecrypt_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	_, err := Decrypt([]byte("short"), "enc:dGVzdA==")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	t.Parallel()

	key := testKey()
	_, err := Decrypt(key, "enc:not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	t.Parallel()

	key := testKey()
	encrypted, err := Encrypt(key, "secret")
	require.NoError(t, err)

	// Tamper with the ciphertext.
	tampered := encrypted[:len(encrypted)-2] + "XX"
	_, err = Decrypt(key, tampered)
	assert.Error(t, err)
}

func TestEncrypt_UniquePerCall(t *testing.T) {
	t.Parallel()

	key := testKey()
	e1, err := Encrypt(key, "same")
	require.NoError(t, err)
	e2, err := Encrypt(key, "same")
	require.NoError(t, err)

	// Different nonces → different ciphertexts.
	assert.NotEqual(t, e1, e2)

	// Both decrypt to the same value.
	d1, err := Decrypt(key, e1)
	require.NoError(t, err)
	d2, err := Decrypt(key, e2)
	require.NoError(t, err)
	assert.Equal(t, d1, d2)
}

func TestIsEncrypted(t *testing.T) {
	t.Parallel()

	assert.True(t, IsEncrypted("enc:abc"))
	assert.False(t, IsEncrypted("plain-value"))
	assert.False(t, IsEncrypted(""))
}
