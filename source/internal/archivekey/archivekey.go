// Package archivekey keeps the game archive password out of plain sight.
//
// A password is stored as one line of text:  SWTLKEY1:<data file>:<base64>   (for example SWTLKEY1:data12:...)
// The text is encrypted (AES-GCM) so it is not readable in the README or the source code, and it can be
// replaced without a new program version when the game changes its password.
//
// This is obfuscation, not real secrecy: the program must be able to decrypt it, so anyone who studies the
// program can too. It only keeps the password from being read, searched for or copied by accident.
package archivekey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

const prefix = "SWTLKEY1:"

func gcm(name string) (cipher.AEAD, error) {
	k := sha256.Sum256([]byte("SW TL Updater archive key v1|" + strings.ToLower(name)))
	b, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// Encode makes the line for one data file name (such as "data12") and its password.
func Encode(name, password string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.ContainsAny(name, ": \t") || password == "" {
		return "", errors.New("need a data file name (like data12) and a password")
	}
	g, err := gcm(name)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := g.Seal(nonce, nonce, []byte(password), []byte(name))
	return prefix + name + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decode reads one line made by Encode.
func Decode(line string) (name, password string, err error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, prefix) {
		return "", "", errors.New("not a key line")
	}
	rest := strings.TrimPrefix(line, prefix)
	i := strings.Index(rest, ":")
	if i <= 0 {
		return "", "", errors.New("key line is incomplete")
	}
	name = strings.ToLower(rest[:i])
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rest[i+1:]))
	if err != nil {
		return "", "", errors.New("key line is damaged (not valid text)")
	}
	g, err := gcm(name)
	if err != nil {
		return "", "", err
	}
	if len(raw) < g.NonceSize()+1 {
		return "", "", errors.New("key line is too short")
	}
	pw, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], []byte(name))
	if err != nil {
		return "", "", errors.New("key line is damaged or copied incompletely")
	}
	return name, string(pw), nil
}

// Find returns the password for one data file from any text: the last valid key line for that name wins,
// so a newer line added below an older one replaces it. Lines for other data files are ignored.
func Find(text, name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	found, ok := "", false
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		if i := strings.Index(ln, prefix); i >= 0 {
			if n, pw, err := Decode(ln[i:]); err == nil && n == name {
				found, ok = pw, true
			}
		}
	}
	return found, ok
}
