package proxyserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const errorPageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%[1]s</title>
<style>
html,body{height:100%%;margin:0}
body{display:flex;align-items:center;justify-content:center;font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;background:#f5f5f5;color:#333}
@media (prefers-color-scheme:dark){body{background:#1e1e1e;color:#ddd}}
main{text-align:center;padding:16px;max-width:480px}
h1{font-size:1.25rem;margin:0 0 .5rem}
p{margin:0;opacity:.8}
</style>
</head>
<body>
<main>
<h1>%[1]s</h1>
<p>%[2]s</p>
</main>
</body>
</html>
`

var errorPages = map[int][]byte{
	http.StatusForbidden: []byte(fmt.Sprintf(errorPageTemplate,
		"Session link is invalid or expired",
		"Please reopen the session from Codio.")),
	http.StatusNotFound: []byte(fmt.Sprintf(errorPageTemplate,
		"Page not found",
		"Please reopen the session from Codio.")),
}

func abortWithErrorPage(c *gin.Context, status int) {
	page, ok := errorPages[status]
	if !ok {
		c.AbortWithStatus(status)
		return
	}
	c.Data(status, "text/html; charset=utf-8", page)
	c.Abort()
}

func frameAncestorsMiddleware(value string) (gin.HandlerFunc, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("frame ancestors value is empty")
	}
	if strings.ContainsAny(value, ";,\r\n") {
		return nil, fmt.Errorf("frame ancestors value %q must not contain ';', ',' or line breaks", value)
	}
	header := "frame-ancestors " + value
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", header)
		c.Next()
	}, nil
}
