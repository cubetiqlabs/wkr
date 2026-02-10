package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid or expired token")

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub  string     `json:"sub"`
	Role model.Role `json:"role"`
	Exp  int64      `json:"exp"`
	Iat  int64      `json:"iat"`
}

func generateJWT(userID uuid.UUID, role model.Role, secret []byte, expiry time.Duration) (string, error) {
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	now := time.Now()
	claims := jwtClaims{
		Sub:  userID.String(),
		Role: role,
		Exp:  now.Add(expiry).Unix(),
		Iat:  now.Unix(),
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	sig := signHS256([]byte(signingInput), secret)

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func ValidateJWT(tokenStr string, secret []byte) (*jwtClaims, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := signHS256([]byte(signingInput), secret)
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrInvalidToken
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("%w: token expired", ErrInvalidToken)
	}

	return &claims, nil
}

func signHS256(data, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return mac.Sum(nil)
}
