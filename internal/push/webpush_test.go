package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := decode64(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// RFC 8291, section 5: the example message, encrypted with its keys and
// salt, gives exactly the RFC's body.
func TestEncryptMatchesRFC8291(t *testing.T) {
	plain := []byte("When I grow up, I want to be a watermelon")
	auth := mustDecode(t, "BTBZMqHH6r4Tts7J_aSIgg")
	uaPublic := mustDecode(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4")
	asPrivate, err := ecdh.P256().NewPrivateKey(mustDecode(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	salt := mustDecode(t, "DGv6ra1nlYgDCS1FRnbzlw")
	got, err := encrypt(plain, uaPublic, auth, asPrivate, salt)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(got) != want {
		t.Errorf("encrypted:\n got %s\nwant %s", b64.EncodeToString(got), want)
	}
	// And the RFC's receiver opens it.
	uaPrivate, err := ecdh.P256().NewPrivateKey(mustDecode(t, "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"))
	if err != nil {
		t.Fatal(err)
	}
	if back := decryptForTest(t, got, uaPrivate, auth); string(back) != string(plain) {
		t.Errorf("decrypted %q", back)
	}
}

// decryptForTest is the receiver's side (RFC 8291 3.4, RFC 8188), here only
// to read what the server sent.
func decryptForTest(t *testing.T, body []byte, ua *ecdh.PrivateKey, auth []byte) []byte {
	t.Helper()
	if len(body) < 21 {
		t.Fatalf("body too short: %d", len(body))
	}
	salt, rs, idlen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	if rs != recordSize || idlen != 65 {
		t.Fatalf("header: rs %d idlen %d", rs, idlen)
	}
	asPublic := body[21 : 21+idlen]
	as, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ua.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}
	info := append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...)
	info = append(info, asPublic...)
	ikm := hmacSHA256(hmacSHA256(auth, shared), append(info, 1))
	prk := hmacSHA256(salt, ikm)
	block, _ := aes.NewCipher(hmacSHA256(prk, []byte("Content-Encoding: aes128gcm\x00\x01"))[:16])
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, hmacSHA256(prk, []byte("Content-Encoding: nonce\x00\x01"))[:12], body[21+idlen:], nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if plain[len(plain)-1] != 2 {
		t.Fatalf("not the last record: %x", plain[len(plain)-1])
	}
	return plain[:len(plain)-1]
}

// The VAPID header: a JWT for the endpoint's origin, signed by the key it
// names (RFC 8292).
func TestVAPID(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	h, err := vapid(key, "https://ntfy.example:8443/upABC?up=1", "https://agendouille.example", now)
	if err != nil {
		t.Fatal(err)
	}
	rest, ok := strings.CutPrefix(h, "vapid t=")
	jwt, k, ok2 := strings.Cut(rest, ", k=")
	if !ok || !ok2 {
		t.Fatalf("header %q", h)
	}
	pub, _ := publicBytes(key)
	if k != b64.EncodeToString(pub) {
		t.Errorf("k is not the key")
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt %q", jwt)
	}
	var claims map[string]any
	if err := json.Unmarshal(mustDecode(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims["aud"] != "https://ntfy.example:8443" || claims["sub"] != "https://agendouille.example" || claims["exp"].(float64) != float64(now.Add(12*time.Hour).Unix()) {
		t.Errorf("claims %v", claims)
	}
	sig := mustDecode(t, parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Errorf("the signature does not verify")
	}
}
