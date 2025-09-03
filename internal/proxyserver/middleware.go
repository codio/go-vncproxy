package proxyserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

type Credentials struct {
	Key  string
	Sign string
}

type Authenticator struct {
	secret               []byte
	credentialsExtractor func(c *gin.Context) Credentials
}

type AuthLocationType int

var AuthLocation = struct {
	Headers     AuthLocationType
	QueryParams AuthLocationType
	PathParams  AuthLocationType
}{
	Headers:     0,
	QueryParams: 1,
	PathParams:  2,
}

var pathsWithoutSign = []string{
	"/ping",
}

func NewAuthenticator(secret string, authLocation AuthLocationType) *Authenticator {
	var extractor func(c *gin.Context) Credentials
	switch authLocation {
	case AuthLocation.Headers:
		extractor = extractFromHeaders
	case AuthLocation.QueryParams:
		extractor = extractFromQueryParams
	case AuthLocation.PathParams:
		extractor = extractFromPathParams
	default:
		extractor = extractFromHeaders
	}
	return &Authenticator{
		secret:               []byte(secret),
		credentialsExtractor: extractor,
	}
}

func extractFromHeaders(c *gin.Context) Credentials {
	return Credentials{
		Key:  c.GetHeader("X-Auth-Key"),
		Sign: c.GetHeader("X-Auth-Sign"),
	}
}

func extractFromQueryParams(c *gin.Context) Credentials {
	return Credentials{
		Key:  c.Query("key"),
		Sign: c.Query("sign"),
	}
}

func extractFromPathParams(c *gin.Context) Credentials {
	return Credentials{
		Key:  c.Param("key"),
		Sign: c.Param("sign"),
	}
}

func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if slices.ContainsFunc(pathsWithoutSign, hasPathPrefix(c.Request.URL.Path)) {
			c.Next()
			return
		}
		credentials := a.credentialsExtractor(c)
		if !a.validateCredentials(credentials) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func hasPathPrefix(path string) func(string) bool {
	return func(prefix string) bool {
		return strings.HasPrefix(path, prefix)
	}
}

func (a *Authenticator) validateCredentials(credentials Credentials) bool {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(credentials.Key))
	expectedMAC := mac.Sum(nil)
	expectedSign := hex.EncodeToString(expectedMAC)
	return credentials.Sign == expectedSign
}
