package gpg

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/packet"
)

// GPG struct represents GPG (GnuPG) service
type GPG struct {
	KeyID          string
	KeyServer      string
	PublicKeyPath  string
	PrivateKeyPath string
	Passphrase     string
}

// New creates GPG provider
func New(publicKeyPath, privateKeyPath, passphrase, keyID, keyServer string) (*GPG, error) {
	return &GPG{
		PublicKeyPath:  publicKeyPath,
		PrivateKeyPath: privateKeyPath,
		Passphrase:     passphrase,
		KeyID:          keyID,
		KeyServer:      keyServer,
	}, nil
}

// Encrypt is responsible for encrypting plaintext and returning ciphertext in bytes using GPG (GnuPG).
// It supports local and remote keys.
// See Crypt.Encrypt
func (p *GPG) Encrypt(plaintext []byte) ([]byte, error) {
	if len(p.PublicKeyPath) > 0 {
		return p.encryptWithKey(plaintext)
	} else if len(p.KeyID) > 0 && len(p.KeyServer) > 0 {
		return p.encryptWithKeyServer(plaintext)
	}
	return nil, errors.New("UNSUPPORTED OPERATION")
}

// Decrypt is responsible for decrypting ciphertext and returning plaintext in bytes using GPG (GnuPG).
// See Crypt.Decrypt
func (p *GPG) Decrypt(ciphertext []byte) ([]byte, error) {
	return p.decryptWithKey(ciphertext)
}

func (p *GPG) encryptWithKeyServer(plaintext []byte) ([]byte, error) {
	keyServer, err := ParseKeyserver(p.KeyServer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse keyserver")
	}
	keyID, err := ParseKeyID(p.KeyID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse key")
	}
	client, err := NewClient(keyServer, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create keyserver client")
	}
	entities, err := client.GetKeysByID(context.TODO(), keyID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get key")
	}
	if len(entities) != 1 {
		return nil, errors.Wrap(err, "more than one entry for the key")
	}
	return p.encrypt(plaintext, entities)
}

func (p *GPG) encryptWithKey(plaintext []byte) ([]byte, error) {
	entity, err := readEntity(p.PublicKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read public key")
	}

	return p.encrypt(plaintext, openpgp.EntityList{entity})
}

func (p *GPG) encrypt(plaintext []byte, entities []*openpgp.Entity) ([]byte, error) {
	buf := new(bytes.Buffer)
	writer, err := openpgp.Encrypt(buf, entities, nil, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encrypt")
	}
	_, err = writer.Write(plaintext)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write ciphertext")
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	encrypted, err := ioutil.ReadAll(buf)
	if err != nil {
		return nil, err
	}

	return encrypted, nil
}

func (p *GPG) decryptWithKey(ciphertext []byte) ([]byte, error) {
	privateKeyEntity, err := readEntity(p.PrivateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read private key")
	}

	if privateKeyEntity.PrivateKey.Encrypted {
		passphraseBytes := []byte(p.Passphrase)
		err = privateKeyEntity.PrivateKey.Decrypt(passphraseBytes)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decrypt private key")
		}
		for _, subkey := range privateKeyEntity.Subkeys {
			err = subkey.PrivateKey.Decrypt(passphraseBytes)
			if err != nil {
				return nil, errors.Wrap(err, "failed to decrypt private key")
			}
		}
	}

	entityList := openpgp.EntityList{privateKeyEntity}
	md, err := openpgp.ReadMessage(bytes.NewBuffer(ciphertext), entityList, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decrypt")
	}
	decrypted, err := ioutil.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read plaintext")
	}

	return decrypted, nil
}

func readEntity(file string) (*openpgp.Entity, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	block, err := armor.Decode(f)
	if err != nil {
		return nil, err
	}
	return openpgp.ReadEntity(packet.NewReader(block.Body))
}
