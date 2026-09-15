// The try box on the website, as one small function.
//
// It runs on demand rather than always: the demo is idle most of the time,
// and a box that costs nothing while nobody is looking is easier to leave
// switched on. The handler is the same one `lamdis try-server` serves, so
// there is one implementation of the limits and one of the record.

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// try is built once and reused across invocations, so the rate limits and
// the sessions survive as long as the execution environment does. Lambda
// may run several of these at once; the credit cap on the key is what
// actually bounds cost, and these only make abuse tedious.
var try *api.Try
var handler http.Handler

func main() {
	model := env("LAMDIS_MODEL", agent.DefaultModel)
	try = &api.Try{
		ModelName:        model,
		Origin:           env("LAMDIS_TRY_ORIGIN", "https://lamdis.ai"),
		PerVisitorPerDay: envInt("LAMDIS_TRY_PER_VISITOR", 8),
		GlobalPerDay:     envInt("LAMDIS_TRY_PER_DAY", 400),
		Logf:             func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) },
	}
	if m := agent.NewOpenRouter(os.Getenv("LAMDIS_TRY_KEY"), model); m != nil {
		try.Model = m
	}
	handler = try.Handler()
	lambda.Start(serve)
}

// serve turns a function-URL request into an ordinary HTTP one, so the
// handler has no idea where it is running.
func serve(ctx context.Context, in events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	body := in.Body
	if in.IsBase64Encoded {
		if raw, err := base64.StdEncoding.DecodeString(body); err == nil {
			body = string(raw)
		}
	}
	method := in.RequestContext.HTTP.Method
	path := in.RawPath
	if path == "" {
		path = "/v1/try"
	}
	// A function URL has no path prefix of its own; anything that is not the
	// demo is the demo anyway, because this function serves nothing else.
	if !strings.HasPrefix(path, "/v1/try") {
		path = "/v1/try"
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}
	if ip := in.RequestContext.HTTP.SourceIP; ip != "" && req.Header.Get("X-Forwarded-For") == "" {
		req.Header.Set("X-Forwarded-For", ip)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))
	res := rec.Result()
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	headers := map[string]string{}
	for k := range res.Header {
		headers[k] = res.Header.Get(k)
	}
	return events.LambdaFunctionURLResponse{
		StatusCode: res.StatusCode,
		Headers:    headers,
		Body:       string(raw),
	}, nil
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(k))); err == nil && v > 0 {
		return v
	}
	return def
}
