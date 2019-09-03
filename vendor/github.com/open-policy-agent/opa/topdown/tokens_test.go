package topdown

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown/internal/jwx/jwk"
	"github.com/open-policy-agent/opa/topdown/internal/jwx/jws"
	"os"
	"strings"
	"testing"
)

func TestParseTokenConstraints(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		var constraints tokenConstraints
		var err error
		c := ast.NewObject()
		constraints, err = parseTokenConstraints(c)
		if err != nil {
			t.Fatalf("parseTokenConstraints: %v", err)
		}
		if constraints.alg != "" {
			t.Errorf("alg: %v", constraints.alg)
		}
		if constraints.key != nil {
			t.Errorf("key: %v", constraints.key)
		}
	})
	t.Run("Alg", func(t *testing.T) {
		var constraints tokenConstraints
		var err error
		c := ast.NewObject()
		c.Insert(ast.StringTerm("alg"), ast.StringTerm("RS256"))
		constraints, err = parseTokenConstraints(c)
		if err != nil {
			t.Fatalf("parseTokenConstraints: %v", err)
		}
		if constraints.alg != "RS256" {
			t.Errorf("alg: %v", constraints.alg)
		}
	})
	t.Run("Cert", func(t *testing.T) {
		var constraints tokenConstraints
		var err error
		c := ast.NewObject()
		c.Insert(ast.StringTerm("cert"), ast.StringTerm(`-----BEGIN CERTIFICATE-----
MIIBcDCCARagAwIBAgIJAMZmuGSIfvgzMAoGCCqGSM49BAMCMBMxETAPBgNVBAMM
CHdoYXRldmVyMB4XDTE4MDgxMDE0Mjg1NFoXDTE4MDkwOTE0Mjg1NFowEzERMA8G
A1UEAwwId2hhdGV2ZXIwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAATPwn3WCEXL
mjp/bFniDwuwsfu7bASlPae2PyWhqGeWwe23Xlyx+tSqxlkXYe4pZ23BkAAscpGj
yn5gXHExyDlKo1MwUTAdBgNVHQ4EFgQUElRjSoVgKjUqY5AXz2o74cLzzS8wHwYD
VR0jBBgwFoAUElRjSoVgKjUqY5AXz2o74cLzzS8wDwYDVR0TAQH/BAUwAwEB/zAK
BggqhkjOPQQDAgNIADBFAiEA4yQ/88ZrUX68c6kOe9G11u8NUaUzd8pLOtkKhniN
OHoCIHmNX37JOqTcTzGn2u9+c8NlnvZ0uDvsd1BmKPaUmjmm
-----END CERTIFICATE-----`))
		constraints, err = parseTokenConstraints(c)
		if err != nil {
			t.Fatalf("parseTokenConstraints: %v", err)
		}
		pubKey := constraints.key.(*ecdsa.PublicKey)
		if pubKey.Curve != elliptic.P256() {
			t.Errorf("curve: %v", pubKey.Curve)
		}
		if pubKey.X.Text(16) != "cfc27dd60845cb9a3a7f6c59e20f0bb0b1fbbb6c04a53da7b63f25a1a86796c1" {
			t.Errorf("x: %x", pubKey.X)
		}
		if pubKey.Y.Text(16) != "edb75e5cb1fad4aac6591761ee29676dc190002c7291a3ca7e605c7131c8394a" {
			t.Errorf("y: %x", pubKey.Y)
		}
	})
	t.Run("Unrecognized", func(t *testing.T) {
		var err error
		c := ast.NewObject()
		c.Insert(ast.StringTerm("hatever"), ast.StringTerm("junk"))
		_, err = parseTokenConstraints(c)
		if err == nil {
			t.Fatalf("parseTokenConstraints: %v", err)
		}
	})
	t.Run("IllFormed", func(t *testing.T) {
		var err error
		c := ast.Array{ast.StringTerm("alg")}
		_, err = parseTokenConstraints(c)
		if err == nil {
			t.Fatalf("parseTokenConstraints: %v", err)
		}
	})
}

func TestParseTokenHeader(t *testing.T) {
	t.Run("Errors", func(t *testing.T) {
		token := &JSONWebToken{
			header: "",
		}
		var err error
		if err = token.decodeHeader(); err == nil {
			t.Fatalf("token.decodeHeader: %v", err)
		}
		token.header = "###"
		if err = token.decodeHeader(); err == nil {
			t.Fatalf("token.decodeHeader: %v", err)
		}
		token.header = base64.RawURLEncoding.EncodeToString([]byte(`{`))
		if err = token.decodeHeader(); err == nil {
			t.Fatalf("token.decodeHeader: %v", err)
		}
		token.header = base64.RawURLEncoding.EncodeToString([]byte(`{}`))
		if err = token.decodeHeader(); err != nil {
			t.Fatalf("token.decodeHeader: %v", err)
		}
		var header tokenHeader
		header, err = parseTokenHeader(token)
		if err != nil {
			t.Fatalf("parseTokenHeader: %v", err)
		}
		if header.valid() {
			t.Fatalf("tokenHeader valid")
		}
	})
	t.Run("Alg", func(t *testing.T) {
		token := &JSONWebToken{
			header: base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)),
		}
		var err error
		if err = token.decodeHeader(); err != nil {
			t.Fatalf("token.decodeHeader: %v", err)
		}
		var header tokenHeader
		header, err = parseTokenHeader(token)
		if err != nil {
			t.Fatalf("parseTokenHeader: %v", err)
		}
		if !header.valid() {
			t.Fatalf("tokenHeader !valid")
		}
		if header.alg != "RS256" {
			t.Fatalf("alg: %s", header.alg)
		}
	})
}

func TestTopDownJWTEncodeSignPayloadErrors(t *testing.T) {

	const examplePayloadError = `{"iss:"joe",` + "\r\n" + ` "exp":1300819380,` + "\r\n" + ` "http://example.com/is_root":true}`
	const hs256Hdr = `{"typ":"JWT",` + "\r\n " + `"alg":"HS256"}`

	params := []struct {
		note   string
		input1 string
		input2 string
		input3 string
		result string
		err    string
	}{
		{
			"No Payload",
			hs256Hdr,
			"",
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"type is JWT but payload is not JSON",
		},
		{
			"Payload JSON Error",
			hs256Hdr,
			examplePayloadError,
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"type is JWT but payload is not JSON",
		},
		{
			"Non JSON Error",
			hs256Hdr,
			"e",
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"type is JWT but payload is not JSON",
		},
	}
	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	var tests []test

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`%s`, p.result)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		tests = append(tests, test{
			p.note,
			[]string{fmt.Sprintf(`p = x { io.jwt.encode_sign_raw(%q, %q, %q, x) }`, p.input1, p.input2, p.input3)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range tests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}

}

func TestTopDownJWTEncodeSignHeaderErrors(t *testing.T) {

	const examplePayload = `{"iss":"joe",` + "\r\n" + ` "exp":1300819380,` + "\r\n" + ` "http://example.com/is_root":true}`
	const hs256HdrError = `{"typ:"JWT",` + "\r\n " + `"alg":"HS256"}`

	params := []struct {
		note   string
		input1 string
		input2 string
		input3 string
		result string
		err    string
	}{
		{
			"Unknown signature algorithm",
			hs256HdrError,
			examplePayload,
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"invalid character",
		},
		{
			"Unknown signature algorithm",
			`{"alg":"dummy"}`,
			examplePayload,
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"Unknown signature algorithm",
		},
		{
			"Empty JSON header Error",
			"{}",
			examplePayload,
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"unsupported signature algorithm",
		},
		{
			"Empty headers input error",
			"",
			examplePayload,
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"unexpected end of JSON input",
		},
		{
			"No JSON Error",
			"e",
			examplePayload,
			`{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`,

			"",
			"invalid character",
		},
	}
	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	var tests []test

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`%s`, p.result)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		tests = append(tests, test{
			p.note,
			[]string{fmt.Sprintf(`p = x { io.jwt.encode_sign_raw(%q, %q, %q, x) }`, p.input1, p.input2, p.input3)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range tests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}
}
func TestTopDownJWTEncodeSignRaw(t *testing.T) {

	const examplePayload = `{"iss":"joe",` + "\r\n" + ` "exp":1300819380,` + "\r\n" + ` "http://example.com/is_root":true}`
	const hs256Hdr = `{"typ":"JWT",` + "\r\n " + `"alg":"HS256"}`
	const rs256Hdr = `{"alg":"RS256"}`
	const hs256HdrPlain = `{"typ":"text/plain",` + "\r\n " + `"alg":"HS256"}`
	const symmetricKey = `{
"kty":"oct",
"k":"AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"
}`
	const rsaKey = `{
    "kty":"RSA",
    "n":"ofgWCuLjybRlzo0tZWJjNiuSfb4p4fAkd_wWJcyQoTbji9k0l8W26mPddxHmfHQp-Vaw-4qPCJrcS2mJPMEzP1Pt0Bm4d4QlL-yRT-SFd2lZS-pCgNMsD1W_YpRPEwOWvG6b32690r2jZ47soMZo9wGzjb_7OMg0LOL-bSf63kpaSHSXndS5z5rexMdbBYUsLA9e-KXBdQOS-UTo7WTBEMa2R2CapHg665xsmtdVMTBQY4uDZlxvb3qCo5ZwKh9kG4LT6_I5IhlJH7aGhyxXFvUK-DWNmoudF8NAco9_h9iaGNj8q2ethFkMLs91kzk2PAcDTW9gb54h4FRWyuXpoQ",
    "e":"AQAB",
    "d":"Eq5xpGnNCivDflJsRQBXHx1hdR1k6Ulwe2JZD50LpXyWPEAeP88vLNO97IjlA7_GQ5sLKMgvfTeXZx9SE-7YwVol2NXOoAJe46sui395IW_GO-pWJ1O0BkTGoVEn2bKVRUCgu-GjBVaYLU6f3l9kJfFNS3E0QbVdxzubSu3Mkqzjkn439X0M_V51gfpRLI9JYanrC4D4qAdGcopV_0ZHHzQlBjudU2QvXt4ehNYTCBr6XCLQUShb1juUO1ZdiYoFaFQT5Tw8bGUl_x_jTj3ccPDVZFD9pIuhLhBOneufuBiB4cS98l2SR_RQyGWSeWjnczT0QU91p1DhOVRuOopznQ",
    "p":"4BzEEOtIpmVdVEZNCqS7baC4crd0pqnRH_5IB3jw3bcxGn6QLvnEtfdUdiYrqBdss1l58BQ3KhooKeQTa9AB0Hw_Py5PJdTJNPY8cQn7ouZ2KKDcmnPGBY5t7yLc1QlQ5xHdwW1VhvKn-nXqhJTBgIPgtldC-KDV5z-y2XDwGUc",
    "q":"uQPEfgmVtjL0Uyyx88GZFF1fOunH3-7cepKmtH4pxhtCoHqpWmT8YAmZxaewHgHAjLYsp1ZSe7zFYHj7C6ul7TjeLQeZD_YwD66t62wDmpe_HlB-TnBA-njbglfIsRLtXlnDzQkv5dTltRJ11BKBBypeeF6689rjcJIDEz9RWdc",
    "dp":"BwKfV3Akq5_MFZDFZCnW-wzl-CCo83WoZvnLQwCTeDv8uzluRSnm71I3QCLdhrqE2e9YkxvuxdBfpT_PI7Yz-FOKnu1R6HsJeDCjn12Sk3vmAktV2zb34MCdy7cpdTh_YVr7tss2u6vneTwrA86rZtu5Mbr1C1XsmvkxHQAdYo0",
    "dq":"h_96-mK1R_7glhsum81dZxjTnYynPbZpHziZjeeHcXYsXaaMwkOlODsWa7I9xXDoRwbKgB719rrmI2oKr6N3Do9U0ajaHF-NKJnwgjMd2w9cjz3_-kyNlxAr2v4IKhGNpmM5iIgOS1VZnOZ68m6_pbLBSp3nssTdlqvd0tIiTHU",
    "qi":"IYd7DHOhrWvxkwPQsRM2tOgrjbcrfvtQJipd-DlcxyVuuM9sQLdgjVk2oy26F0EmpScGLq2MowX7fhd_QJQ3ydy5cY7YIBi87w93IKLEdfnbJtoOPLUW0ITrJReOgo1cq9SbsxYawBgfp_gh6A5603k2-ZQwVK0JKSHuLFkuQ3U"
  }`

	params := []struct {
		note   string
		input1 string
		input2 string
		input3 string
		result string
		err    string
	}{
		{
			"https://tools.ietf.org/html/rfc7515#appendix-A.1",
			"`" + hs256Hdr + "`",
			"`" + examplePayload + "`",
			"`" + symmetricKey + "`",

			`"eyJ0eXAiOiJKV1QiLA0KICJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJqb2UiLA0KICJleHAiOjEzMDA4MTkzODAsDQogImh0dHA6Ly9leGFtcGxlLmNvbS9pc19yb290Ijp0cnVlfQ.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"`,
			"",
		},
		{
			"No Payload but Media Type is Plain",
			"`" + hs256HdrPlain + "`",
			"`" + "" + "`",
			"`" + symmetricKey + "`",

			`"eyJ0eXAiOiJ0ZXh0L3BsYWluIiwNCiAiYWxnIjoiSFMyNTYifQ..sXoGQMWwM-SmX495-htA7kndgbkwz1PnqsDeY275gnI""`,
			"",
		},
		{
			"text/plain media type",
			"`" + hs256HdrPlain + "`",
			"`" + "e" + "`",
			"`" + symmetricKey + "`",

			`"eyJ0eXAiOiJ0ZXh0L3BsYWluIiwNCiAiYWxnIjoiSFMyNTYifQ.ZQ.oO8Vnc4Jv7-J231a1bEcQrgXfKbNW-kEvVY7BP1v5rM""`,
			"",
		},
		{
			"Empty JSON payload",
			"`" + hs256Hdr + "`",
			"`" + "{}" + "`",
			"`" + symmetricKey + "`",

			`"eyJ0eXAiOiJKV1QiLA0KICJhbGciOiJIUzI1NiJ9.e30.KAml6HRetE0sq22SYNh_CQExhf-X31ChYTfGwUBIWu8"`,
			"",
		},
		{
			"https://tools.ietf.org/html/rfc7515#appendix-A.2",
			"`" + rs256Hdr + "`",
			"`" + examplePayload + "`",
			"`" + rsaKey + "`",

			`"eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJqb2UiLA0KICJleHAiOjEzMDA4MTkzODAsDQogImh0dHA6Ly9leGFtcGxlLmNvbS9pc19yb290Ijp0cnVlfQ.cC4hiUPoj9Eetdgtv3hF80EGrhuB__dzERat0XF9g2VtQgr9PJbu3XOiZj5RZmh7AAuHIm4Bh-0Qc_lF5YKt_O8W2Fp5jujGbds9uJdbF9CUAr7t1dnZcAcQjbKBYNX4BAynRFdiuB--f_nZLgrnbyTyWzO75vRK5h6xBArLIARNPvkSjtQBMHlb1L07Qe7K0GarZRmB_eSN9383LcOLn6_dO--xi12jzDwusC-eOkHWEsqtFZESc6BfI7noOPqvhJ1phCnvWh6IeYI2w9QOYEUipUTI8np6LbgGY9Fs98rqVt5AXLIhWkWywlVmtVrBp0igcN_IoypGlUPQGe77Rw"`,
			"",
		},
	}
	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	var rawTests []test

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`%s`, p.result)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		rawTests = append(rawTests, test{
			p.note,
			[]string{fmt.Sprintf(`p = x { io.jwt.encode_sign_raw(%s, %s, %s, x) }`, p.input1, p.input2, p.input3)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range rawTests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}

}

func TestTopDownJWTEncodeSignES256(t *testing.T) {

	const examplePayload = `{"iss":"joe",` + "\r\n" + ` "exp":1300819380,` + "\r\n" + ` "http://example.com/is_root":true}`
	const es256Hdr = `{"alg":"ES256"}`
	const ecKey = `{
    "kty":"EC",
    "crv":"P-256",
    "x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU",
    "y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0",
    "d":"jpsQnnGQmL-YBIffH1136cspYG6-0iY7X1fCE9-E9LI"
  }`

	params := struct {
		note   string
		input1 string
		input2 string
		input3 string
		err    string
	}{

		"https://tools.ietf.org/html/rfc7515#appendix-A.3",
		"`" + es256Hdr + "`",
		"`" + examplePayload + "`",
		"`" + ecKey + "`",

		"",
	}
	type test struct {
		note  string
		rules []string
	}

	tc := test{
		params.note,
		[]string{fmt.Sprintf(`p = x { io.jwt.encode_sign_raw(%s, %s, %s, x) }`, params.input1, params.input2, params.input3)},
	}

	compiler, err := compileRules(nil, tc.rules, nil)

	if err != nil {
		t.Errorf("%v: Compiler error: %v", tc.note, err)
		return
	}
	store := inmem.New()
	path := []string{"p"}
	var inputTerm *ast.Term

	ctx := context.Background()
	txn := storage.NewTransactionOrDie(ctx, store)

	defer store.Abort(ctx, txn)

	var lhs *ast.Term
	if len(path) == 0 {
		lhs = ast.NewTerm(ast.DefaultRootRef)
	} else {
		lhs = ast.MustParseTerm("data." + strings.Join(path, "."))
	}

	rhs := ast.VarTerm(ast.WildcardPrefix + "result")
	body := ast.NewBody(ast.Equality.Expr(lhs, rhs))

	query := NewQuery(body).
		WithCompiler(compiler).
		WithStore(store).
		WithTransaction(txn).
		WithInput(inputTerm)

	var tracer BufferTracer

	if os.Getenv("OPA_TRACE_TEST") != "" {
		query = query.WithTracer(&tracer)
	}

	qrs, err := query.Run(ctx)

	if tracer != nil {
		PrettyTrace(os.Stdout, tracer)
	}

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(qrs) == 0 {
		t.Fatal("Undefined result")
	}

	result, err := ast.JSON(qrs[0][rhs.Value.(ast.Var)].Value)
	if err != nil {
		t.Fatal(err)
	}
	// Verification

	standardHeaders := &jws.StandardHeaders{}
	err = json.Unmarshal([]byte(es256Hdr), standardHeaders)
	if err != nil {
		t.Fatal("Failed to parse header")
	}
	alg := standardHeaders.GetAlgorithm()

	keys, err := jwk.ParseString(ecKey)
	if err != nil {
		t.Fatal("Failed to parse JWK")
	}
	key, err := keys.Keys[0].Materialize()
	if err != nil {
		t.Fatal("Failed to create private key")
	}
	publicKey, err := jwk.GetPublicKey(key)

	// Verify with vendor library

	verifiedPayload, err := jws.Verify([]byte(result.(string)), alg, publicKey)
	if err != nil || string(verifiedPayload) != examplePayload {
		t.Fatal("Failed to verify message")
	}
}

// TestTopDownJWTEncodeSignEC needs to perform all tests inline because we do not know the
// expected values before hand
func TestTopDownJWTEncodeSignES512(t *testing.T) {

	const examplePayload = `{"iss":"joe",` + "\r\n" + ` "exp":1300819380,` + "\r\n" + ` "http://example.com/is_root":true}`
	const es512Hdr = `{"alg":"ES512"}`
	const ecKey = `{
"kty":"EC",
"crv":"P-521",
"x":"AekpBQ8ST8a8VcfVOTNl353vSrDCLLJXmPk06wTjxrrjcBpXp5EOnYG_NjFZ6OvLFV1jSfS9tsz4qUxcWceqwQGk",
"y":"ADSmRA43Z1DSNx_RvcLI87cdL07l6jQyyBXMoxVg_l2Th-x3S1WDhjDly79ajL4Kkd0AZMaZmh9ubmf63e3kyMj2",
"d":"AY5pb7A0UFiB3RELSD64fTLOSV_jazdF7fLYyuTw8lOfRhWg6Y6rUrPAxerEzgdRhajnu0ferB0d53vM9mE15j2C"
}`

	params := struct {
		note   string
		input1 string
		input2 string
		input3 string
		err    string
	}{

		"https://tools.ietf.org/html/rfc7515#appendix-A.4",
		"`" + es512Hdr + "`",
		"`" + examplePayload + "`",
		"`" + ecKey + "`",

		"",
	}
	type test struct {
		note  string
		rules []string
	}
	var tests []test

	tests = append(tests, test{
		params.note,
		[]string{fmt.Sprintf(`p = x { io.jwt.encode_sign_raw(%s, %s, %s, x) }`, params.input1, params.input2, params.input3)},
	})

	tc := tests[0]

	compiler, err := compileRules(nil, tc.rules, nil)

	if err != nil {
		t.Errorf("%v: Compiler error: %v", tc.note, err)
		return
	}
	store := inmem.New()
	path := []string{"p"}
	var inputTerm *ast.Term

	ctx := context.Background()
	txn := storage.NewTransactionOrDie(ctx, store)

	defer store.Abort(ctx, txn)

	var lhs *ast.Term
	if len(path) == 0 {
		lhs = ast.NewTerm(ast.DefaultRootRef)
	} else {
		lhs = ast.MustParseTerm("data." + strings.Join(path, "."))
	}

	rhs := ast.VarTerm(ast.WildcardPrefix + "result")
	body := ast.NewBody(ast.Equality.Expr(lhs, rhs))

	query := NewQuery(body).
		WithCompiler(compiler).
		WithStore(store).
		WithTransaction(txn).
		WithInput(inputTerm)

	var tracer BufferTracer

	if os.Getenv("OPA_TRACE_TEST") != "" {
		query = query.WithTracer(&tracer)
	}

	qrs, err := query.Run(ctx)

	if tracer != nil {
		PrettyTrace(os.Stdout, tracer)
	}

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(qrs) == 0 {
		t.Fatal("Undefined result")
	}

	result, err := ast.JSON(qrs[0][rhs.Value.(ast.Var)].Value)
	if err != nil {
		t.Fatal(err)
	}
	// Verification

	standardHeaders := &jws.StandardHeaders{}
	err = json.Unmarshal([]byte(es512Hdr), standardHeaders)
	if err != nil {
		t.Fatal("Failed to parse header")
	}
	alg := standardHeaders.GetAlgorithm()

	keys, err := jwk.ParseString(ecKey)
	if err != nil {
		t.Fatal("Failed to parse JWK")
	}
	key, err := keys.Keys[0].Materialize()
	if err != nil {
		t.Fatal("Failed to create private key")
	}
	publicKey, err := jwk.GetPublicKey(key)

	// Verify with vendor library

	verifiedPayload, err := jws.Verify([]byte(result.(string)), alg, publicKey)
	if err != nil || string(verifiedPayload) != examplePayload {
		t.Fatal("Failed to verify message")
	}
}

func TestTopDownJWTBuiltins(t *testing.T) {
	params := []struct {
		note      string
		input     string
		header    string
		payload   string
		signature string
		err       string
	}{
		{
			"simple",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwIiwiaXNzIjoib3BhIn0.XmVoLoHI3pxMtMO_WRONMSJzGUDP9pDjy8Jp0_tdRXY`,
			`{ "alg": "HS256", "typ": "JWT" }`,
			`{ "sub": "0", "iss": "opa" }`,
			`5e65682e81c8de9c4cb4c3bf59138d3122731940cff690e3cbc269d3fb5d4576`,
			"",
		},
		{
			"simple-non-registered",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuZXciOiJJIGFtIGEgdXNlciBjcmVhdGVkIGZpZWxkIiwiaXNzIjoib3BhIn0.6UmjsclVDGD9jcmX_F8RJzVgHtUZuLu2pxkF_UEQCrE`,
			`{ "alg": "HS256", "typ": "JWT" }`,
			`{ "new": "I am a user created field", "iss": "opa" }`,
			`e949a3b1c9550c60fd8dc997fc5f112735601ed519b8bbb6a71905fd41100ab1`,
			"",
		},
		{
			"no-support-jwe",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImVuYyI6ImJsYWgifQ.eyJuZXciOiJJIGFtIGEgdXNlciBjcmVhdGVkIGZpZWxkIiwiaXNzIjoib3BhIn0.McGUb1e-UviZKy6UyQErNNQzEUgeV25Buwk7OHOa8U8`,
			``,
			``,
			``,
			"JWT is a JWE object, which is not supported",
		},
		{
			"no-periods",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9eyJzdWIiOiIwIiwiaXNzIjoib3BhIn0XmVoLoHI3pxMtMO_WRONMSJzGUDP9pDjy8Jp0_tdRXY`,
			``,
			``,
			``,
			"encoded JWT had no period separators",
		},
		{
			"wrong-period-count",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXV.CJ9eyJzdWIiOiIwIiwiaXNzIjoib3BhIn0XmVoLoHI3pxMtMO_WRONMSJzGUDP9pDjy8Jp0_tdRXY`,
			``,
			``,
			``,
			"encoded JWT must have 3 sections, found 2",
		},
		{
			"bad-header-encoding",
			`eyJhbGciOiJIU^%zI1NiI+sInR5cCI6IkpXVCJ9.eyJzdWIiOiIwIiwiaXNzIjoib3BhIn0.XmVoLoHI3pxMtMO_WRONMSJzGUDP9pDjy8Jp0_tdRXY`,
			``,
			``,
			``,
			"JWT header had invalid encoding: illegal base64 data at input byte 13",
		},
		{
			"bad-payload-encoding",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwIiwia/XNzIjoib3BhIn0.XmVoLoHI3pxMtMO_WRONMSJzGUDP9pDjy8Jp0_tdRXY`,
			``,
			``,
			``,
			"JWT payload had invalid encoding: illegal base64 data at input byte 17",
		},
		{
			"bad-signature-encoding",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwIiwiaXNzIjoib3BhIn0.XmVoLoHI3pxMtMO(_WRONMSJzGUDP9pDjy8Jp0_tdRXY`,
			``,
			``,
			``,
			"JWT signature had invalid encoding: illegal base64 data at input byte 15",
		},
		{
			"nested",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImN0eSI6IkpXVCJ9.ImV5SmhiR2NpT2lKSVV6STFOaUlzSW5SNWNDSTZJa3BYVkNKOS5leUp6ZFdJaU9pSXdJaXdpYVhOeklqb2liM0JoSW4wLlhtVm9Mb0hJM3B4TXRNT19XUk9OTVNKekdVRFA5cERqeThKcDBfdGRSWFki.8W0qx4mLxslmZl7wEMUWBxH7tST3XsEuWXxesXqFnRI`,
			`{ "alg": "HS256", "typ": "JWT" }`,
			`{ "sub": "0", "iss": "opa" }`,
			`5e65682e81c8de9c4cb4c3bf59138d3122731940cff690e3cbc269d3fb5d4576`,
			"",
		},
		{
			"double-nested",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImN0eSI6IkpXVCJ9.ImV5SmhiR2NpT2lKSVV6STFOaUlzSW5SNWNDSTZJa3BYVkNJc0ltTjBlU0k2SWtwWFZDSjkuSW1WNVNtaGlSMk5wVDJsS1NWVjZTVEZPYVVselNXNVNOV05EU1RaSmEzQllWa05LT1M1bGVVcDZaRmRKYVU5cFNYZEphWGRwWVZoT2VrbHFiMmxpTTBKb1NXNHdMbGh0Vm05TWIwaEpNM0I0VFhSTlQxOVhVazlPVFZOS2VrZFZSRkE1Y0VScWVUaEtjREJmZEdSU1dGa2kuOFcwcXg0bUx4c2xtWmw3d0VNVVdCeEg3dFNUM1hzRXVXWHhlc1hxRm5SSSI.U8rwnGAJ-bJoGrAYKEzNtbJQWd3x1eW0Y25nLKHDCgo`,
			`{ "alg": "HS256", "typ": "JWT" }`,
			`{ "sub": "0", "iss": "opa" }`,
			`5e65682e81c8de9c4cb4c3bf59138d3122731940cff690e3cbc269d3fb5d4576`,
			"",
		},
		{
			"complex-values",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwIiwiaXNzIjoib3BhIiwiZXh0Ijp7ImFiYyI6IjEyMyIsImNiYSI6WzEwLCIxMCJdfX0.IIxF-uJ6i4K5Dj71xNLnUeqB9jmujl6ujTInhii1PxE`,
			`{ "alg": "HS256", "typ": "JWT" }`,
			`{ "sub": "0", "iss": "opa", "ext": { "abc": "123", "cba": [10, "10"] } }`,
			`208c45fae27a8b82b90e3ef5c4d2e751ea81f639ae8e5eae8d32278628b53f11`,
			"",
		},
		// The test below checks that payloads with duplicate keys
		// in their encoding produce a token object that binds the key
		// to the last occurring value, as per RFC 7519 Section 4.
		// It tests a payload encoding that has 3 duplicates of the
		// "iss" key, with the values "not opa", "also not opa" and
		// "opa", in that order.
		// Go's json.Unmarshal exhibits this behavior, but it is not
		// documented, so this test is meant to catch that behavior
		// if it changes.
		{
			"duplicate-keys",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiAiMCIsImlzcyI6ICJub3Qgb3BhIiwgImlzcyI6ICJhbHNvIG5vdCBvcGEiLCAiaXNzIjogIm9wYSJ9.XmVoLoHI3pxMtMO_WRONMSJzGUDP9pDjy8Jp0_tdRXY`,
			`{ "alg": "HS256", "typ": "JWT" }`,
			`{ "sub": "0", "iss": "opa" }`,
			`5e65682e81c8de9c4cb4c3bf59138d3122731940cff690e3cbc269d3fb5d4576`,
			"",
		},
	}

	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	tests := []test{}

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`[%s, %s, "%s"]`, p.header, p.payload, p.signature)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		tests = append(tests, test{
			p.note,
			[]string{fmt.Sprintf(`p = [x, y, z] { io.jwt.decode("%s", [x, y, z]) }`, p.input)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range tests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}
}

const certPem = `-----BEGIN CERTIFICATE-----\nMIIFiDCCA3ACCQCGV6XsfG/oRTANBgkqhkiG9w0BAQUFADCBhTELMAkGA1UEBhMC\nVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExFTATBgNVBAcMDFJlZHdvb2QgQ2l0eTEO\nMAwGA1UECgwFU3R5cmExDDAKBgNVBAsMA0RldjESMBAGA1UEAwwJbG9jYWxob3N0\nMRgwFgYJKoZIhvcNAQkBFglhc2hAc3R5cmEwHhcNMTgwMzA2MDAxNTU5WhcNMTkw\nMzA2MDAxNTU5WjCBhTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWEx\nFTATBgNVBAcMDFJlZHdvb2QgQ2l0eTEOMAwGA1UECgwFU3R5cmExDDAKBgNVBAsM\nA0RldjESMBAGA1UEAwwJbG9jYWxob3N0MRgwFgYJKoZIhvcNAQkBFglhc2hAc3R5\ncmEwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQDucnAwTRA0zqDQ671L\nKWOVwhjhycFyzyhZUd7vhsnslOBiYM6TYIDXhETfAk2RQoRE/9xF16woMD8FOglc\nlSuhi+GNfFRif6LfArm84ZFj1ZS1MX2logikhXhRJQ7AOHe5+ED0re3KH5lWyqfz\nR6bQuPYwTQSBJy6Tq7T9RiOM29yadCX64OaCEbzEFmHtNlbb5px4zCVvgskg/fpV\nGGCMpAYjGDatbxE5eAloVs1EJuI5RSqWr1JRm6EejxM04BFdfGn1HgWrsKXtlvBa\n00/AC0zXL5n6LK7+L3WbRguVTZcE4Yu70gDwhmM+VsKeT9LKClX003BNj0NJDRB9\ndw9MaWxsXDNHNOWEfbnASXeP7ZRv3D81ftij6P8SL14ZnxyrRty8TAN4ij3wd41l\nastRQCtrJFi+HzO606XOp6HDzBoWT0DGl8Sn2hZ6RLPyBnD04vvvcSGeCVjHGOQ8\nc3OTroK58u5MR/q4T00sTkeeVAxuKoEWKsjIBYYrJTe/a2mEq9yiDGbPNYDnWnQZ\njSUZm+Us23Y2sm/agZ5zKXcEuoecGL6sYCixr/xeB9BPxEiTthH+0M8OY99qpIhz\nSmj41wdgQfzZi/6B8pIr77V/KywYKxJEmzw8Uy48aC/rZ8WsT8QdKwclo1aiNJhx\n79OvGbZFoeHD/w7igpx+ttpF/wIDAQABMA0GCSqGSIb3DQEBBQUAA4ICAQC3wWUs\nfXz+aSfFVz+O3mLFkr65NIgazbGAySgMgMNVuadheIkPL4k21atyflfpx4pg9FGv\n40vWCLMajpvynfz4oqah0BACnpqzQ8Dx6HYkmlXK8fLB+WtPrZBeUEsGPKuJYt4M\nd5TeY3VpNgWOPXmnE4lvxHZqh/8OwmOpjBfC9E3e2eqgwiwOkXnMaZEPgKP6JiWk\nEFaQ9jgMQqJZnNcv6NmiqqsZeI0/NNjBpkmEWQl+wLegVusHiQ0FMBMQ0taEo21r\nzUwHoNJR3h3wgGQiKxKOH1FUKHBV7hEqObLraD/hfG5xYucJfvvAAP1iH0ycPs+9\nhSccrn5/HY1c9AZnW8Kh7atp/wFP+sHjtECWK/lUmXfhASS293hprCpJk2n9pkmR\nziXKJhjwkxlC8NcHuiVfaxdfDa4+1Qta2gK7GEypbvLoEmIt/dsYUsxUg84lwJJ9\nnyC/pfZ5a8wFSf186JeVH4kHd3bnkzlQz460HndOMSJ/Xi1wSfuZlOVupFf8TVKl\np4j28MTLH2Wqx50NssKThdaX6hoCiMqreYa+EVaN1f/cIGQxZSCzdzMCKqdB8lKB\n3Eax+5zsIa/UyPwGxZcyXBRHAlz5ZnkjuRxInyiMkBWWz3IZXjTe6Fq8BNd2UWNc\nw35+2nO5n1LKXgR2+nzhZUOk8TPsi9WUywRluQ==\n-----END CERTIFICATE-----`
const certPemPs = `-----BEGIN CERTIFICATE-----\nMIIC/DCCAeSgAwIBAgIJAJRvYDU3ei3EMA0GCSqGSIb3DQEBCwUAMBMxETAPBgNV\nBAMMCHdoYXRldmVyMB4XDTE4MDgxMDEwMzgxNloXDTE4MDkwOTEwMzgxNlowEzER\nMA8GA1UEAwwId2hhdGV2ZXIwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB\nAQC4kCmzLMW/5jzkzkmN7Me8wPD+ymBUIjsGqliGfMrfFfDV2eTPVtZcYD3IXoB4\nAOUT7XJzWjOsBRFOcVKKEiCPjXiLcwLb/QWQ1x0Budft32r3+N0KQd1rgcRHTPNc\nJoeWCfOgDPp51RTzTT6HQuV4ud+CDhRJP7QMVMIgal9Nuzs49LLZaBPW8/rFsHjk\nJQ4kDujSrpcT6F2FZY3SmWsOJgP7RjVKk5BheYeFKav5ZV4p6iHn/TN4RVpvpNBh\n5z/XoHITJ6lpkHSDpbIaQUTpobU2um8N3biz+HsEAmD9Laa27WUpYSpiM6DDMSXl\ndBDJdumerVRJvXYCtfXqtl17AgMBAAGjUzBRMB0GA1UdDgQWBBRz74MkVzT2K52/\nFJC4mTa9coM/DTAfBgNVHSMEGDAWgBRz74MkVzT2K52/FJC4mTa9coM/DTAPBgNV\nHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAD1ZE4IaIAetqGG+vt9oz1\nIx0j4EPok0ONyhhmiSsF6rSv8zlNWweVf5y6Z+AoTNY1Fym0T7dbpbqIox0EdKV3\nFLzniWOjznupbnqfXwHX/g1UAZSyt3akSatVhvNpGlnd7efTIAiNinX/TkzIjhZ7\nihMIZCGykT1P0ys1OaeEf57wAzviatD4pEMTIW0OOqY8bdRGhuJR1kKUZ/2Nm8Ln\ny7E0y8uODVbH9cAwGyzWB/QFc+bffNgi9uJaPQQc5Zxwpu9utlqyzFvXgV7MBYUK\nEYSLyxp4g4e5aujtLugaC8H6n9vP1mEBr/+T8HGynBZHNTKlDhhL9qDbpkkNB6/w\n-----END CERTIFICATE-----`
const certPemEs256 = `-----BEGIN CERTIFICATE-----\nMIIBcDCCARagAwIBAgIJAMZmuGSIfvgzMAoGCCqGSM49BAMCMBMxETAPBgNVBAMM\nCHdoYXRldmVyMB4XDTE4MDgxMDE0Mjg1NFoXDTE4MDkwOTE0Mjg1NFowEzERMA8G\nA1UEAwwId2hhdGV2ZXIwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAATPwn3WCEXL\nmjp/bFniDwuwsfu7bASlPae2PyWhqGeWwe23Xlyx+tSqxlkXYe4pZ23BkAAscpGj\nyn5gXHExyDlKo1MwUTAdBgNVHQ4EFgQUElRjSoVgKjUqY5AXz2o74cLzzS8wHwYD\nVR0jBBgwFoAUElRjSoVgKjUqY5AXz2o74cLzzS8wDwYDVR0TAQH/BAUwAwEB/zAK\nBggqhkjOPQQDAgNIADBFAiEA4yQ/88ZrUX68c6kOe9G11u8NUaUzd8pLOtkKhniN\nOHoCIHmNX37JOqTcTzGn2u9+c8NlnvZ0uDvsd1BmKPaUmjmm\n-----END CERTIFICATE-----\n`
const certPemBad = `-----BEGIN CERT-----\nMIIFiDCCA3ACCQCGV6XsfG/oRTANBgkqhkiG9w0BAQUFADCBhTELMAkGA1UEBhMC\nVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExFTATBgNVBAcMDFJlZHdvb2QgQ2l0eTEO\nMAwGA1UECgwFU3R5cmExDDAKBgNVBAsMA0RldjESMBAGA1UEAwwJbG9jYWxob3N0\nMRgwFgYJKoZIhvcNAQkBFglhc2hAc3R5cmEwHhcNMTgwMzA2MDAxNTU5WhcNMTkw\nMzA2MDAxNTU5WjCBhTELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWEx\nFTATBgNVBAcMDFJlZHdvb2QgQ2l0eTEOMAwGA1UECgwFU3R5cmExDDAKBgNVBAsM\nA0RldjESMBAGA1UEAwwJbG9jYWxob3N0MRgwFgYJKoZIhvcNAQkBFglhc2hAc3R5\ncmEwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQDucnAwTRA0zqDQ671L\nKWOVwhjhycFyzyhZUd7vhsnslOBiYM6TYIDXhETfAk2RQoRE/9xF16woMD8FOglc\nlSuhi+GNfFRif6LfArm84ZFj1ZS1MX2logikhXhRJQ7AOHe5+ED0re3KH5lWyqfz\nR6bQuPYwTQSBJy6Tq7T9RiOM29yadCX64OaCEbzEFmHtNlbb5px4zCVvgskg/fpV\nGGCMpAYjGDatbxE5eAloVs1EJuI5RSqWr1JRm6EejxM04BFdfGn1HgWrsKXtlvBa\n00/AC0zXL5n6LK7+L3WbRguVTZcE4Yu70gDwhmM+VsKeT9LKClX003BNj0NJDRB9\ndw9MaWxsXDNHNOWEfbnASXeP7ZRv3D81ftij6P8SL14ZnxyrRty8TAN4ij3wd41l\nastRQCtrJFi+HzO606XOp6HDzBoWT0DGl8Sn2hZ6RLPyBnD04vvvcSGeCVjHGOQ8\nc3OTroK58u5MR/q4T00sTkeeVAxuKoEWKsjIBYYrJTe/a2mEq9yiDGbPNYDnWnQZ\njSUZm+Us23Y2sm/agZ5zKXcEuoecGL6sYCixr/xeB9BPxEiTthH+0M8OY99qpIhz\nSmj41wdgQfzZi/6B8pIr77V/KywYKxJEmzw8Uy48aC/rZ8WsT8QdKwclo1aiNJhx\n79OvGbZFoeHD/w7igpx+ttpF/wIDAQABMA0GCSqGSIb3DQEBBQUAA4ICAQC3wWUs\nfXz+aSfFVz+O3mLFkr65NIgazbGAySgMgMNVuadheIkPL4k21atyflfpx4pg9FGv\n40vWCLMajpvynfz4oqah0BACnpqzQ8Dx6HYkmlXK8fLB+WtPrZBeUEsGPKuJYt4M\nd5TeY3VpNgWOPXmnE4lvxHZqh/8OwmOpjBfC9E3e2eqgwiwOkXnMaZEPgKP6JiWk\nEFaQ9jgMQqJZnNcv6NmiqqsZeI0/NNjBpkmEWQl+wLegVusHiQ0FMBMQ0taEo21r\nzUwHoNJR3h3wgGQiKxKOH1FUKHBV7hEqObLraD/hfG5xYucJfvvAAP1iH0ycPs+9\nhSccrn5/HY1c9AZnW8Kh7atp/wFP+sHjtECWK/lUmXfhASS293hprCpJk2n9pkmR\nziXKJhjwkxlC8NcHuiVfaxdfDa4+1Qta2gK7GEypbvLoEmIt/dsYUsxUg84lwJJ9\nnyC/pfZ5a8wFSf186JeVH4kHd3bnkzlQz460HndOMSJ/Xi1wSfuZlOVupFf8TVKl\np4j28MTLH2Wqx50NssKThdaX6hoCiMqreYa+EVaN1f/cIGQxZSCzdzMCKqdB8lKB\n3Eax+5zsIa/UyPwGxZcyXBRHAlz5ZnkjuRxInyiMkBWWz3IZXjTe6Fq8BNd2UWNc\nw35+2nO5n1LKXgR2+nzhZUOk8TPsi9WUywRluQ==\n-----END CERT-----`

func TestTopDownJWTVerifyRSA(t *testing.T) {

	params := []struct {
		note   string
		alg    string
		input1 string
		input2 string
		result bool
		err    string
	}{
		{
			"success",
			"rs256",
			`eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJuYmYiOjE0NDQ0Nzg0MDB9.N0-EVdv5pvUfZYFRzMGnsWpNLHgwMEgViPwpuLBEtt32682OgnOK-N4X-2gpQEjQIbUr0IFym8YsRQU9GZvqQP72Sd6yOQNGSNeE74DpUZCAjBa9SBIb1UlD2MxZB-e7YJiEyo7pZhimaqorXrgorlaXYGMvsCFWDYmBLzGaGYaGJyEpkZHzHb7ujsDrJJjdEtDV3kh13gTHzLPvqnoXuuxelXye_8LPIhvgDy52gT4shUEso71pJCMv_IqAR19ljVE17lJzoi6VhRn6ReNUE-yg4KfCO4Ypnuu-mcQr7XtmSYoWkX72L5UQ-EyWkoz-w0SYKoJTPzHkTL2thYStksVpeNkGuck25aUdtrQgmPbao0QOWBFlkg03e6mPCD2-aXOt1ofth9mZGjxWMHX-mUqHaNmaWM3WhRztJ73hWrmB1YOdYQtOEHejfvR_td5tqIw4W6ufRy2ScOypGQe7kNaUZxpgxZ1927ZGNiQgawIOAQwXOcFx1JNSEIeg55-cYJrHPxsXGOB9ZxW-qnswmFJp474iUVXjzGhLexJDXBwvKGs_O3JFjMsvyV9_hm7bnQU0vG_HgPYs5i9VOHRMujq1vFBcm52TFVOBGdWaGfb9RRdLLYvVkJLk0Poh19rsCWb7-Vc3mAaGGpvuk4Wv-PnGGNC-V-FQqIbijHDrn_g`,
			certPem,
			true,
			"",
		},
		{
			"success-ps256",
			"ps256",
			`eyJ0eXAiOiAiSldUIiwgImFsZyI6ICJQUzI1NiJ9.eyJuYmYiOiAxNDQ0NDc4NDAwLCAiZm9vIjogImJhciJ9.i0F3MHWzOsBNLqjQzK1UVeQid9xPMowCoUsoM-C2BDxUY-FMKmCeJ1NJ4TGnS9HzFK1ftEvRnPT7EOxOkHPoCk1rz3feTFgtHtNzQqLM1IBTnz6aHHOrda_bKPHH9ZIYCRQUPXhpC90ivW_IJR-f7Z1WLrMXaJ71i1XteruENHrJJJDn0HedHG6N0VHugBHrak5k57cbE31utAdx83TEd8v2Y8wAkCJXKrdmTa-8419LNxW_yjkvoDD53n3X5CHhYkSymU77p0v6yWO38qDWeKJ-Fm_PrMAo72_rizDBj_yPa5LA3bT_EnsgZtC-sp8_SCDIH41bjiCGpRHhqgZmyw`,
			certPemPs,
			true,
			"",
		},
		{
			"success-es256",
			"es256",
			`eyJ0eXAiOiAiSldUIiwgImFsZyI6ICJFUzI1NiJ9.eyJuYmYiOiAxNDQ0NDc4NDAwLCAiaXNzIjogInh4eCJ9.lArczfN-pIL8oUU-7PU83u-zfXougXBZj6drFeKFsPEoVhy9WAyiZlRshYqjTSXdaw8yw2L-ovt4zTUZb2PWMg`,
			certPemEs256,
			true,
			"",
		},
		{
			"failure-bad token",
			"rs256",
			`eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJuYmYiOjE0NDQ0Nzg0MDB9.Yt89BjaPCNgol478rYyH66-XgkHos02TsVwxLH3ZlvOoIVjbhYW8q1_MHehct1-yBf1UOX3g-lUrIjpoDtX1TfAESuaWTjYPixRvjfJ-Nn75JF8QuAl5PD27C6aJ4PjUPNfj0kwYBnNQ_oX-ZFb781xRi7qRDB6swE4eBUxzHqKUJBLaMM2r8k1-9iE3ERNeqTJUhV__p0aSyRj-i62rdZ4TC5nhxtWodiGP4e4GrYlXkdaKduK63cfdJF-kfZfTsoDs_xy84pZOkzlflxuNv9bNqd-3ISAdWe4gsEvWWJ8v70-QWkydnH8rhj95DaqoXrjfzbOgDpKtdxJC4daVPKvntykzrxKhZ9UtWzm3OvJSKeyWujFZlldiTfBLqNDgdi-Boj_VxO5Pdh-67lC3L-pBMm4BgUqf6rakBQvoH7AV6zD5CbFixh7DuqJ4eJHHItWzJwDctMrV3asm-uOE1E2B7GErGo3iX6S9Iun_kvRUp6kyvOaDq5VvXzQOKyLQIQyHGGs0aIV5cFI2IuO5Rt0uUj5mzPQrQWHgI4r6Mc5bzmq2QLxBQE8OJ1RFhRpsuoWQyDM8aRiMQIJe1g3x4dnxbJK4dYheYblKHFepScYqT1hllDp3oUNn89sIjQIhJTe8KFATu4K8ppluys7vhpE2a_tq8i5O0MFxWmsxN4Q`,
			certPem,
			false,
			"",
		},
		{
			"failure-wrong key",
			"ps256",
			`eyJ0eXAiOiAiSldUIiwgImFsZyI6ICJQUzI1NiJ9.eyJuYmYiOiAxNDQ0NDc4NDAwLCAiZm9vIjogImJhciJ9.i0F3MHWzOsBNLqjQzK1UVeQid9xPMowCoUsoM-C2BDxUY-FMKmCeJ1NJ4TGnS9HzFK1ftEvRnPT7EOxOkHPoCk1rz3feTFgtHtNzQqLM1IBTnz6aHHOrda_bKPHH9ZIYCRQUPXhpC90ivW_IJR-f7Z1WLrMXaJ71i1XteruENHrJJJDn0HedHG6N0VHugBHrak5k57cbE31utAdx83TEd8v2Y8wAkCJXKrdmTa-8419LNxW_yjkvoDD53n3X5CHhYkSymU77p0v6yWO38qDWeKJ-Fm_PrMAo72_rizDBj_yPa5LA3bT_EnsgZtC-sp8_SCDIH41bjiCGpRHhqgZmyw`,
			certPem,
			false,
			"",
		},
		{
			"failure-wrong alg",
			"ps256",
			`eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJuYmYiOjE0NDQ0Nzg0MDB9.N0-EVdv5pvUfZYFRzMGnsWpNLHgwMEgViPwpuLBEtt32682OgnOK-N4X-2gpQEjQIbUr0IFym8YsRQU9GZvqQP72Sd6yOQNGSNeE74DpUZCAjBa9SBIb1UlD2MxZB-e7YJiEyo7pZhimaqorXrgorlaXYGMvsCFWDYmBLzGaGYaGJyEpkZHzHb7ujsDrJJjdEtDV3kh13gTHzLPvqnoXuuxelXye_8LPIhvgDy52gT4shUEso71pJCMv_IqAR19ljVE17lJzoi6VhRn6ReNUE-yg4KfCO4Ypnuu-mcQr7XtmSYoWkX72L5UQ-EyWkoz-w0SYKoJTPzHkTL2thYStksVpeNkGuck25aUdtrQgmPbao0QOWBFlkg03e6mPCD2-aXOt1ofth9mZGjxWMHX-mUqHaNmaWM3WhRztJ73hWrmB1YOdYQtOEHejfvR_td5tqIw4W6ufRy2ScOypGQe7kNaUZxpgxZ1927ZGNiQgawIOAQwXOcFx1JNSEIeg55-cYJrHPxsXGOB9ZxW-qnswmFJp474iUVXjzGhLexJDXBwvKGs_O3JFjMsvyV9_hm7bnQU0vG_HgPYs5i9VOHRMujq1vFBcm52TFVOBGdWaGfb9RRdLLYvVkJLk0Poh19rsCWb7-Vc3mAaGGpvuk4Wv-PnGGNC-V-FQqIbijHDrn_g`,
			certPem,
			false,
			"",
		},
		{
			"failure-invalid token",
			"rs256",
			`eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJuYmYiOjE0NDQ0Nzg0MDB9`,
			certPem,
			false,
			"encoded JWT must have 3 sections, found 2",
		},
		{
			"failure-bad cert",
			"rs256",
			`eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJuYmYiOjE0NDQ0Nzg0MDB9.N0-EVdv5pvUfZYFRzMGnsWpNLHgwMEgViPwpuLBEtt32682OgnOK-N4X-2gpQEjQIbUr0IFym8YsRQU9GZvqQP72Sd6yOQNGSNeE74DpUZCAjBa9SBIb1UlD2MxZB-e7YJiEyo7pZhimaqorXrgorlaXYGMvsCFWDYmBLzGaGYaGJyEpkZHzHb7ujsDrJJjdEtDV3kh13gTHzLPvqnoXuuxelXye_8LPIhvgDy52gT4shUEso71pJCMv_IqAR19ljVE17lJzoi6VhRn6ReNUE-yg4KfCO4Ypnuu-mcQr7XtmSYoWkX72L5UQ-EyWkoz-w0SYKoJTPzHkTL2thYStksVpeNkGuck25aUdtrQgmPbao0QOWBFlkg03e6mPCD2-aXOt1ofth9mZGjxWMHX-mUqHaNmaWM3WhRztJ73hWrmB1YOdYQtOEHejfvR_td5tqIw4W6ufRy2ScOypGQe7kNaUZxpgxZ1927ZGNiQgawIOAQwXOcFx1JNSEIeg55-cYJrHPxsXGOB9ZxW-qnswmFJp474iUVXjzGhLexJDXBwvKGs_O3JFjMsvyV9_hm7bnQU0vG_HgPYs5i9VOHRMujq1vFBcm52TFVOBGdWaGfb9RRdLLYvVkJLk0Poh19rsCWb7-Vc3mAaGGpvuk4Wv-PnGGNC-V-FQqIbijHDrn_g`,
			certPemBad,
			false,
			"failed to decode PEM block containing certificate",
		},
	}

	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	tests := []test{}

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`%t`, p.result)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		tests = append(tests, test{
			p.note,
			[]string{fmt.Sprintf(`p = x { io.jwt.verify_%s("%s", "%s", x) }`, p.alg, p.input1, p.input2)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range tests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}
}

func TestTopDownJWTVerifyHS256(t *testing.T) {
	params := []struct {
		note   string
		input1 string
		input2 string
		result bool
		err    string
	}{
		{
			"success",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWxpY2UiLCJhenAiOiJhbGljZSIsInN1Ym9yZGluYXRlcyI6W10sImhyIjpmYWxzZX0.rz3jTY033z-NrKfwrK89_dcLF7TN4gwCMj-fVBDyLoM`,
			"secret",
			true,
			"",
		},
		{
			"failure-bad token",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWxpY2UiLCJhenAiOiJhbGljZSIsInN1Ym9yZGluYXRlcyI6W10sImhyIjpmYWxzZX0.R0NDxM1gHTucWQKwayMDre2PbMNR9K9efmOfygDZWcE`,
			"secret",
			false,
			"",
		},
		{
			"failure-invalid token",
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWxpY2UiLCJhenAiOiJhbGljZSIsInN1Ym9yZGluYXRlcyI6W10sImhyIjpmYWxzZX0`,
			"secret",
			false,
			"encoded JWT must have 3 sections, found 2",
		},
	}

	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	tests := []test{}

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`%t`, p.result)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		tests = append(tests, test{
			p.note,
			[]string{fmt.Sprintf(`p = x { io.jwt.verify_hs256("%s", "%s", x) }`, p.input1, p.input2)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range tests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}
}

func TestTopDownJWTDecodeVerify(t *testing.T) {
	params := []struct {
		note        string // test name
		token       string // JWT
		constraints string // constraints argument
		valid       bool   // expected validity value
		header      string // expected header
		payload     string // expected claims
		err         string // expected error or "" for succes
	}{
		{
			"ps256-unconstrained", // no constraints at all (apart from supplying a key)
			"eyJhbGciOiAiUFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.iCePYnD1U13oBe_6ylhmojmkY_VZNYXqVszAej8RImMGv51OEqARmYFkRZYTiYCiVFober7vcDq_stOj1uAJCuttygGW_dpHiN-3EWsU2E2vCnXlygWe0ud38pOC-OVyEFbXxO9-m51vnS-3VmBjEO8G1UE8bLFXTeFOGkUIj9dqlefJSWh5wa8XA3g9mj0jqpuJi-7QgEIeVHk-JzhGpoFqI2f-Df_agVvc2x4V-6fJmj7wV2IsaFPRi36mVQmg8S-dkxu4AlaeCILhyNZl8ewjBHHBjJFRwzcy88L00mzdO51ZxEYsBdQav3ux2sc6vjT9PvvjAwzcthQxEoEaNA",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			true,
			`{"alg": "PS256", "typ": "JWT"}`,
			`{"iss": "xxx"}`,
			"",
		},
		{
			"ps256-key-wrong", // wrong key for signature
			"eyJhbGciOiAiUFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.iCePYnD1U13oBe_6ylhmojmkY_VZNYXqVszAej8RImMGv51OEqARmYFkRZYTiYCiVFober7vcDq_stOj1uAJCuttygGW_dpHiN-3EWsU2E2vCnXlygWe0ud38pOC-OVyEFbXxO9-m51vnS-3VmBjEO8G1UE8bLFXTeFOGkUIj9dqlefJSWh5wa8XA3g9mj0jqpuJi-7QgEIeVHk-JzhGpoFqI2f-Df_agVvc2x4V-6fJmj7wV2IsaFPRi36mVQmg8S-dkxu4AlaeCILhyNZl8ewjBHHBjJFRwzcy88L00mzdO51ZxEYsBdQav3ux2sc6vjT9PvvjAwzcthQxEoEaNA",
			fmt.Sprintf(`{"cert": "%s"}`, certPem),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-key-wrong", // wrong key for signature
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImV4cCI6IDMwMDB9.hqDP3AzshNhUZMI02U3nLPrj93QFrgs-74XFrF1Vry2bplrz-NKpdVdfTu8iY_bhmkWf2Om5DdwRZj2ZgpGahtnshnHaRq0RyqF-m3Y7oNj6JL_YMwgxsFIIHtBlagBqDU-gZK99iqSOSGqVhvxqX6gCqFgE7vnEGHeeDedtRM53coAJuwzy8rQV9m3TewoofPdPasGv-dBLQZ3qgmnibkSgb7SmFpjXBy8zL3xJXOZhAHYlgcmcEoFVaWlBguIcWA87WZlpCLYcdYTJzSZweC3QLUhZ4RLJW84-LMKp6xWLLPrp3OgnsduB2G9PYMmYw_qCkuY1KGwfH4PvCQbAzQ",
			fmt.Sprintf(`{"cert": "%s"}`, certPem),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"ps256-iss-ok", // enforce issuer
			"eyJhbGciOiAiUFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.iCePYnD1U13oBe_6ylhmojmkY_VZNYXqVszAej8RImMGv51OEqARmYFkRZYTiYCiVFober7vcDq_stOj1uAJCuttygGW_dpHiN-3EWsU2E2vCnXlygWe0ud38pOC-OVyEFbXxO9-m51vnS-3VmBjEO8G1UE8bLFXTeFOGkUIj9dqlefJSWh5wa8XA3g9mj0jqpuJi-7QgEIeVHk-JzhGpoFqI2f-Df_agVvc2x4V-6fJmj7wV2IsaFPRi36mVQmg8S-dkxu4AlaeCILhyNZl8ewjBHHBjJFRwzcy88L00mzdO51ZxEYsBdQav3ux2sc6vjT9PvvjAwzcthQxEoEaNA",
			fmt.Sprintf(`{"cert": "%s", "iss": "xxx"}`, certPemPs),
			true,
			`{"alg": "PS256", "typ": "JWT"}`,
			`{"iss": "xxx"}`,
			"",
		},
		{
			"ps256-iss-wrong", // wrong issuer
			"eyJhbGciOiAiUFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.iCePYnD1U13oBe_6ylhmojmkY_VZNYXqVszAej8RImMGv51OEqARmYFkRZYTiYCiVFober7vcDq_stOj1uAJCuttygGW_dpHiN-3EWsU2E2vCnXlygWe0ud38pOC-OVyEFbXxO9-m51vnS-3VmBjEO8G1UE8bLFXTeFOGkUIj9dqlefJSWh5wa8XA3g9mj0jqpuJi-7QgEIeVHk-JzhGpoFqI2f-Df_agVvc2x4V-6fJmj7wV2IsaFPRi36mVQmg8S-dkxu4AlaeCILhyNZl8ewjBHHBjJFRwzcy88L00mzdO51ZxEYsBdQav3ux2sc6vjT9PvvjAwzcthQxEoEaNA",
			fmt.Sprintf(`{"cert": "%s", "iss": "yyy"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"ps256-alg-ok", // constrained algorithm
			"eyJhbGciOiAiUFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.iCePYnD1U13oBe_6ylhmojmkY_VZNYXqVszAej8RImMGv51OEqARmYFkRZYTiYCiVFober7vcDq_stOj1uAJCuttygGW_dpHiN-3EWsU2E2vCnXlygWe0ud38pOC-OVyEFbXxO9-m51vnS-3VmBjEO8G1UE8bLFXTeFOGkUIj9dqlefJSWh5wa8XA3g9mj0jqpuJi-7QgEIeVHk-JzhGpoFqI2f-Df_agVvc2x4V-6fJmj7wV2IsaFPRi36mVQmg8S-dkxu4AlaeCILhyNZl8ewjBHHBjJFRwzcy88L00mzdO51ZxEYsBdQav3ux2sc6vjT9PvvjAwzcthQxEoEaNA",
			fmt.Sprintf(`{"cert": "%s", "alg": "PS256"}`, certPemPs),
			true,
			`{"alg": "PS256", "typ": "JWT"}`,
			`{"iss": "xxx"}`,
			"",
		},
		{
			"ps256-alg-wrong", // constrained algorithm, and it's wrong
			"eyJhbGciOiAiUFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.iCePYnD1U13oBe_6ylhmojmkY_VZNYXqVszAej8RImMGv51OEqARmYFkRZYTiYCiVFober7vcDq_stOj1uAJCuttygGW_dpHiN-3EWsU2E2vCnXlygWe0ud38pOC-OVyEFbXxO9-m51vnS-3VmBjEO8G1UE8bLFXTeFOGkUIj9dqlefJSWh5wa8XA3g9mj0jqpuJi-7QgEIeVHk-JzhGpoFqI2f-Df_agVvc2x4V-6fJmj7wV2IsaFPRi36mVQmg8S-dkxu4AlaeCILhyNZl8ewjBHHBjJFRwzcy88L00mzdO51ZxEYsBdQav3ux2sc6vjT9PvvjAwzcthQxEoEaNA",
			fmt.Sprintf(`{"cert": "%s", "alg": "RS256"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-exp-ok", // token expires, and it's still valid
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImV4cCI6IDMwMDB9.hqDP3AzshNhUZMI02U3nLPrj93QFrgs-74XFrF1Vry2bplrz-NKpdVdfTu8iY_bhmkWf2Om5DdwRZj2ZgpGahtnshnHaRq0RyqF-m3Y7oNj6JL_YMwgxsFIIHtBlagBqDU-gZK99iqSOSGqVhvxqX6gCqFgE7vnEGHeeDedtRM53coAJuwzy8rQV9m3TewoofPdPasGv-dBLQZ3qgmnibkSgb7SmFpjXBy8zL3xJXOZhAHYlgcmcEoFVaWlBguIcWA87WZlpCLYcdYTJzSZweC3QLUhZ4RLJW84-LMKp6xWLLPrp3OgnsduB2G9PYMmYw_qCkuY1KGwfH4PvCQbAzQ",
			fmt.Sprintf(`{"cert": "%s", "time": 2000000000000}`, certPemPs),
			true,
			`{"alg": "RS256", "typ": "JWT"}`,
			`{"iss": "xxx", "exp": 3000}`,
			"",
		},
		{
			"rs256-exp-expired", // token expires, and it's stale at a chosen time
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImV4cCI6IDMwMDB9.hqDP3AzshNhUZMI02U3nLPrj93QFrgs-74XFrF1Vry2bplrz-NKpdVdfTu8iY_bhmkWf2Om5DdwRZj2ZgpGahtnshnHaRq0RyqF-m3Y7oNj6JL_YMwgxsFIIHtBlagBqDU-gZK99iqSOSGqVhvxqX6gCqFgE7vnEGHeeDedtRM53coAJuwzy8rQV9m3TewoofPdPasGv-dBLQZ3qgmnibkSgb7SmFpjXBy8zL3xJXOZhAHYlgcmcEoFVaWlBguIcWA87WZlpCLYcdYTJzSZweC3QLUhZ4RLJW84-LMKp6xWLLPrp3OgnsduB2G9PYMmYw_qCkuY1KGwfH4PvCQbAzQ",
			fmt.Sprintf(`{"cert": "%s", "time": 4000000000000}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-exp-now-expired", // token expires, and it's stale at the current implicitly specified real time
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImV4cCI6IDMwMDB9.hqDP3AzshNhUZMI02U3nLPrj93QFrgs-74XFrF1Vry2bplrz-NKpdVdfTu8iY_bhmkWf2Om5DdwRZj2ZgpGahtnshnHaRq0RyqF-m3Y7oNj6JL_YMwgxsFIIHtBlagBqDU-gZK99iqSOSGqVhvxqX6gCqFgE7vnEGHeeDedtRM53coAJuwzy8rQV9m3TewoofPdPasGv-dBLQZ3qgmnibkSgb7SmFpjXBy8zL3xJXOZhAHYlgcmcEoFVaWlBguIcWA87WZlpCLYcdYTJzSZweC3QLUhZ4RLJW84-LMKp6xWLLPrp3OgnsduB2G9PYMmYw_qCkuY1KGwfH4PvCQbAzQ",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-exp-now-explicit-expired", // token expires, and it's stale at the current explicitly specified real time
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImV4cCI6IDMwMDB9.hqDP3AzshNhUZMI02U3nLPrj93QFrgs-74XFrF1Vry2bplrz-NKpdVdfTu8iY_bhmkWf2Om5DdwRZj2ZgpGahtnshnHaRq0RyqF-m3Y7oNj6JL_YMwgxsFIIHtBlagBqDU-gZK99iqSOSGqVhvxqX6gCqFgE7vnEGHeeDedtRM53coAJuwzy8rQV9m3TewoofPdPasGv-dBLQZ3qgmnibkSgb7SmFpjXBy8zL3xJXOZhAHYlgcmcEoFVaWlBguIcWA87WZlpCLYcdYTJzSZweC3QLUhZ4RLJW84-LMKp6xWLLPrp3OgnsduB2G9PYMmYw_qCkuY1KGwfH4PvCQbAzQ",
			fmt.Sprintf(`{"cert": "%s", "time": now}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-nbf-ok", // token has a commencement time, and it's commenced at a chosen time
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJuYmYiOiAxMDAwLCAiaXNzIjogInh4eCJ9.cwwYDfJhU_ambPIpwBJwDek05miffoudprr41IAYsl0IKekb1ii2uEgwkNM-LJtVXHe9hsK3gANFyfqoJuCZIBvaNMx_3Z0BUdeBs4k1UwBiZCpuud0ofgHKURwvehNgqDvRfchq_-K_Agi2iRdl0oShgLjN-gVbBl8pRwUbQrvASlcsCpZIKUyOzXNtaIZEFh1z6ISDy8UHHOdoieKpN23swya7QAcEb0wXEEKMkkhiRd5QHgWLk37Lnw2K89mKcq4Om0CtV9nHrxxmpYGSMPojCy16Gjdg5-xKyJWvxCfb3YUBUVM4RWa7ICOPRJWPuHxu9pPYG63hb_qDU6NLsw",
			fmt.Sprintf(`{"cert": "%s", "time": 2000000000000}`, certPemPs),
			true,
			`{"alg": "RS256", "typ": "JWT"}`,
			`{"iss": "xxx", "nbf": 1000}`,
			"",
		},
		{
			"rs256-nbf-now-ok", // token has a commencement time, and it's commenced at the current implicitly specified time
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJuYmYiOiAxMDAwLCAiaXNzIjogInh4eCJ9.cwwYDfJhU_ambPIpwBJwDek05miffoudprr41IAYsl0IKekb1ii2uEgwkNM-LJtVXHe9hsK3gANFyfqoJuCZIBvaNMx_3Z0BUdeBs4k1UwBiZCpuud0ofgHKURwvehNgqDvRfchq_-K_Agi2iRdl0oShgLjN-gVbBl8pRwUbQrvASlcsCpZIKUyOzXNtaIZEFh1z6ISDy8UHHOdoieKpN23swya7QAcEb0wXEEKMkkhiRd5QHgWLk37Lnw2K89mKcq4Om0CtV9nHrxxmpYGSMPojCy16Gjdg5-xKyJWvxCfb3YUBUVM4RWa7ICOPRJWPuHxu9pPYG63hb_qDU6NLsw",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			true,
			`{"alg": "RS256", "typ": "JWT"}`,
			`{"iss": "xxx", "nbf": 1000}`,
			"",
		},
		{
			"rs256-nbf-toosoon", // token has a commencement time, and the chosen time is too early
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJuYmYiOiAxMDAwLCAiaXNzIjogInh4eCJ9.cwwYDfJhU_ambPIpwBJwDek05miffoudprr41IAYsl0IKekb1ii2uEgwkNM-LJtVXHe9hsK3gANFyfqoJuCZIBvaNMx_3Z0BUdeBs4k1UwBiZCpuud0ofgHKURwvehNgqDvRfchq_-K_Agi2iRdl0oShgLjN-gVbBl8pRwUbQrvASlcsCpZIKUyOzXNtaIZEFh1z6ISDy8UHHOdoieKpN23swya7QAcEb0wXEEKMkkhiRd5QHgWLk37Lnw2K89mKcq4Om0CtV9nHrxxmpYGSMPojCy16Gjdg5-xKyJWvxCfb3YUBUVM4RWa7ICOPRJWPuHxu9pPYG63hb_qDU6NLsw",
			fmt.Sprintf(`{"cert": "%s", "time": 500000000000}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-alg-missing", // alg is missing from the JOSE header
			"eyJ0eXAiOiAiSldUIiwgImtpZCI6ICJrMSJ9.eyJpc3MiOiAieHh4IiwgInN1YiI6ICJmcmVkIn0.J4J4FgUD_P5fviVVjgvQWJDg-5XYTP_tHCwB3kSlYVKv8vmnZRNh4ke68OxfMP96iM-LZswG2fNqe-_piGIMepF5rCe1iIWAuz3qqkxfS9YVF3hvwoXhjJT0yIgrDMl1lfW5_XipNshZoxddWK3B7dnVW74MFazEEFuefiQm3PdMUX8jWGsmfgPnqBIZTizErNhoIMuRvYaVM1wA2nfrpVGONxMTaw8T0NRwYIuZwubbnNQ1yLhI0y3dsZvQ_lrh9Khtk9fS1V3SRh7aa9AvferJ4T-48qn_V1m3sINPgoA-uLGyyu3k_GkXRYW1yGNC-MH4T2cwhj89WITbIhusgQ",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-crit-junk", // the JOSE header contains an unrecognized critical parameter
			"eyJjcml0IjogWyJqdW5rIl0sICJraWQiOiAiazEiLCAiYWxnIjogIlJTMjU2IiwgInR5cCI6ICJKV1QiLCAianVuayI6ICJ4eHgifQ.eyJpc3MiOiAieHh4IiwgInN1YiI6ICJmcmVkIn0.YfoUpW5CgDBtxtBuOix3cdYJGT8cX9Mq7wOhIbjDK7eRQUsAmMY_0EQPh7bd7Yi1gLI3e11BKzguf2EHqAa1kbkHWwFniBO-RIi8q42v2uxC4lpEpIjfaaXB5XmsLfAXtYRqh0AObvbSho6VDXBP_Kn81nhIiE2yFbH14_jhRMSxDBs5ToSkXV-XJHw5bONP8NxPqEk9KF3ZJGzN7J_KoD6LjqfYai5K0eLNEIZh4C1WjTdmCKMR4K6ieZRQWZiSsnhSqLSQERir4n22G3QsdY7dOnCp-SS4VYu3V-PfsOSFMvQ-TTAN1geqMZ9A7k1CCLW0wxKBs-KCiYzmRTzwxA",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rsa256-nested", // one nesting level
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCIsICJjdHkiOiAiSldUIn0.ZXlKaGJHY2lPaUFpVWxNeU5UWWlMQ0FpZEhsd0lqb2dJa3BYVkNKOS5leUpwYzNNaU9pQWllSGg0SW4wLnJSUnJlUU9DYW9ZLW1Nazcyak5GZVk1YVlFUWhJZ0lFdFZkUTlYblltUUwyTHdfaDdNbkk0U0VPMVBwa0JIVEpyZnljbEplTHpfalJ2UGdJMlcxaDFCNGNaVDhDZ21pVXdxQXI5c0puZHlVQ1FtSWRrbm53WkI5cXAtX3BTdGRHWEo5WnAzeEo4NXotVEJpWlN0QUNUZFdlUklGSUU3VkxPa20tRmxZdzh5OTdnaUN4TmxUdWl3amxlTjMwZDhnWHUxNkZGQzJTSlhtRjZKbXYtNjJHbERhLW1CWFZ0bGJVSTVlWVUwaTdueTNyQjBYUVQxRkt4ZUZ3OF85N09FdV9jY3VLcl82ZHlHZVFHdnQ5Y3JJeEFBMWFZbDdmbVBrNkVhcjllTTNKaGVYMi00Wkx0d1FOY1RDT01YV0dIck1DaG5MWVc4WEFrTHJEbl9yRmxUaVMtZw.Xicc2sWCZ_Nithucsw9XD7YOKrirUdEnH3MyiPM-Ck3vEU2RsTBsfU2JPhfjp3phc0VOgsAXCzwU5PwyNyUo1490q8YSym-liMyO2Lk-hjH5fAxoizg9yD4II_lK6Wz_Tnpc0bBGDLdbuUhvgvO7yqo-leBQlsfRXOvw4VSPSEy8QPtbURtbnLpWY2jGBKz7vGI_o4qDJ3PicG0kyEiWZNh3wjeeCYRCWvXN8qh7Uk5EA-8J5vX651GqV-7gmaX1n-8DXamhaCQcE-p1cjSj04-X-_bJlQtmb-TT3bSyUPxgHVncvxNUby8jkUTzfi5MMbmIzWWkxI5YtJTdtmCkPQ",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			true,
			`{"alg": "RS256", "typ": "JWT"}`,
			`{"iss": "xxx"}`,
			"",
		},
		{
			"rsa256-nested2", // two nesting levels
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCIsICJjdHkiOiAiSldUIn0.ZXlKaGJHY2lPaUFpVWxNeU5UWWlMQ0FpZEhsd0lqb2dJa3BYVkNJc0lDSmpkSGtpT2lBaVNsZFVJbjAuWlhsS2FHSkhZMmxQYVVGcFZXeE5lVTVVV1dsTVEwRnBaRWhzZDBscWIyZEphM0JZVmtOS09TNWxlVXB3WXpOTmFVOXBRV2xsU0dnMFNXNHdMbkpTVW5KbFVVOURZVzlaTFcxTmF6Y3lhazVHWlZrMVlWbEZVV2hKWjBsRmRGWmtVVGxZYmxsdFVVd3lUSGRmYURkTmJrazBVMFZQTVZCd2EwSklWRXB5Wm5samJFcGxUSHBmYWxKMlVHZEpNbGN4YURGQ05HTmFWRGhEWjIxcFZYZHhRWEk1YzBwdVpIbFZRMUZ0U1dScmJtNTNXa0k1Y1hBdFgzQlRkR1JIV0VvNVduQXplRW80TlhvdFZFSnBXbE4wUVVOVVpGZGxVa2xHU1VVM1ZreFBhMjB0Um14WmR6aDVPVGRuYVVONFRteFVkV2wzYW14bFRqTXdaRGhuV0hVeE5rWkdRekpUU2xodFJqWktiWFl0TmpKSGJFUmhMVzFDV0ZaMGJHSlZTVFZsV1ZVd2FUZHVlVE55UWpCWVVWUXhSa3Q0WlVaM09GODVOMDlGZFY5alkzVkxjbDgyWkhsSFpWRkhkblE1WTNKSmVFRkJNV0ZaYkRkbWJWQnJOa1ZoY2psbFRUTkthR1ZZTWkwMFdreDBkMUZPWTFSRFQwMVlWMGRJY2sxRGFHNU1XVmM0V0VGclRISkVibDl5Um14VWFWTXRady5YaWNjMnNXQ1pfTml0aHVjc3c5WEQ3WU9LcmlyVWRFbkgzTXlpUE0tQ2szdkVVMlJzVEJzZlUySlBoZmpwM3BoYzBWT2dzQVhDendVNVB3eU55VW8xNDkwcThZU3ltLWxpTXlPMkxrLWhqSDVmQXhvaXpnOXlENElJX2xLNld6X1RucGMwYkJHRExkYnVVaHZndk83eXFvLWxlQlFsc2ZSWE92dzRWU1BTRXk4UVB0YlVSdGJuTHBXWTJqR0JLejd2R0lfbzRxREozUGljRzBreUVpV1pOaDN3amVlQ1lSQ1d2WE44cWg3VWs1RUEtOEo1dlg2NTFHcVYtN2dtYVgxbi04RFhhbWhhQ1FjRS1wMWNqU2owNC1YLV9iSmxRdG1iLVRUM2JTeVVQeGdIVm5jdnhOVWJ5OGprVVR6Zmk1TU1ibUl6V1dreEk1WXRKVGR0bUNrUFE.ODBVH_gooCLJxtPVr1MjJC1syG4MnVUFP9LkI9pSaj0QABV4vpfqrBshHn8zOPgUTDeHwbc01Qy96cQlTMQQb94YANmZyL1nzwmdR4piiGXMGSlcCNfDg1o8DK4msMSR-X-j2IkxBDB8rfeFSfLRMgDCjAF0JolW7qWmMD9tBmFNYAjly4vMwToOXosDmFLl5eqyohXDf-3Ohljm5kIjtyMWkt5S9EVuwlIXh2owK5l59c4-TH29gkuaZ3uU4LFPjD7XKUrlOQnEMuu2QD8LAqTyxbnY4JyzUWEvyTM1dVmGnFpLKCg9QBly__y1u2ffhvDsHyuCmEKAbhPE98YvFA",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			true,
			`{"alg": "RS256", "typ": "JWT"}`,
			`{"iss": "xxx"}`,
			"",
		},
		{
			"es256-unconstrained", // ECC key, no constraints
			"eyJhbGciOiAiRVMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.JvbTLBF06FR70gb7lCbx_ojhp4bk9--B_aULgNlYM0fYf9OSawaqBQp2lwW6FADFtRJ2WFUk5g0zwVOUlnrlzw",
			fmt.Sprintf(`{"cert": "%s"}`, certPemEs256),
			true,
			`{"alg": "ES256", "typ": "JWT"}`,
			`{"iss": "xxx"}`,
			"",
		},
		{
			"hs256-unconstrained", // HMAC key, no constraints
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWxpY2UiLCJhenAiOiJhbGljZSIsInN1Ym9yZGluYXRlcyI6W10sImhyIjpmYWxzZX0.rz3jTY033z-NrKfwrK89_dcLF7TN4gwCMj-fVBDyLoM`,
			`{"secret": "secret"}`,
			true,
			`{"alg": "HS256", "typ": "JWT"}`,
			`{"user": "alice", "azp": "alice", "subordinates": [], "hr": false}`,
			"",
		},
		{
			"hs256-key-wrong", // HMAC with wrong key
			`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWxpY2UiLCJhenAiOiJhbGljZSIsInN1Ym9yZGluYXRlcyI6W10sImhyIjpmYWxzZX0.rz3jTY033z-NrKfwrK89_dcLF7TN4gwCMj-fVBDyLoM`,
			`{"secret": "the wrong key"}`,
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-aud", // constraint requires an audience, found right one in JWT
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImF1ZCI6ICJmcmVkIn0.F-9m2Tx8r1tuQFirazsI4FK05bXX3uP4ut8M2FryJ07k3bQhy262fdwNDmuFcGx0NfL-c80agcwGoTzMWXkVEgZ2KTz0QSAdcdGk3ZWtUy-Mj2IilZ1dzkVvW8LsithYFTGcUtkelFDrJwtMQ0Kum7SXJpC_HCBk4PbftY0XD6jRgHLnQdeT9_J11L4sd19vCdpxxxm3_m_yvUV3ZynzB4vhQbS3CET4EClAVhi-m_gMh9mj85gY1ycIz6-FxWv8xM2Igm2SMeIdyJwAvEGnIauRS928P_OqVCZgCH2Pafnxtzy77Llpxy8XS0xu5PtPw3_azhg33GaXDCFsfz6GpA",
			fmt.Sprintf(`{"cert": "%s", "aud": "fred"}`, certPemPs),
			true,
			`{"alg": "RS256", "typ": "JWT"}`,
			`{"aud": "fred", "iss": "xxx"}`,
			"",
		},
		{
			"rs256-aud-list", // constraint requires an audience, found list including right one in JWT
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImF1ZCI6IFsiZnJlZCIsICJib2IiXX0.k8jW7PUiMkQCKCjnSFBFFKPDO0RXwZgVkLUwUfi8sMdrrcKi12LC8wd5fLBn0YraFtMXWKdMweKf9ZC-K33h5TK7kkTVKOXctF50mleMlUn0Up_XjtdP1v-2WOfivUXcexN1o-hu0kH7sSQnielXIjC2EAleG6A54YUOZFBdzvd1PKHlsxA7x2iiL73uGeFlyxoaMki8E5tx7FY6JGF1RdhWCoIV5A5J8QnwI5EetduJQ505U65Pk7UApWYWu4l2DT7KCCJa5dJaBvCBemVxWaBhCQWtJKU2ZgOEkpiK7b_HsdeRBmpG9Oi1o5mt5ybC09VxSD-lEda_iJO_7i042A",
			fmt.Sprintf(`{"cert": "%s", "aud": "bob"}`, certPemPs),
			true,
			`{"alg": "RS256", "typ": "JWT"}`,
			`{"aud": ["fred", "bob"], "iss": "xxx"}`,
			"",
		},
		{
			"ps256-no-aud", // constraint requires an audience, none in JWT
			"eyJhbGciOiAiUFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4In0.iCePYnD1U13oBe_6ylhmojmkY_VZNYXqVszAej8RImMGv51OEqARmYFkRZYTiYCiVFober7vcDq_stOj1uAJCuttygGW_dpHiN-3EWsU2E2vCnXlygWe0ud38pOC-OVyEFbXxO9-m51vnS-3VmBjEO8G1UE8bLFXTeFOGkUIj9dqlefJSWh5wa8XA3g9mj0jqpuJi-7QgEIeVHk-JzhGpoFqI2f-Df_agVvc2x4V-6fJmj7wV2IsaFPRi36mVQmg8S-dkxu4AlaeCILhyNZl8ewjBHHBjJFRwzcy88L00mzdO51ZxEYsBdQav3ux2sc6vjT9PvvjAwzcthQxEoEaNA",
			fmt.Sprintf(`{"cert": "%s", "aud": "cath"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-missing-aud", // constraint requires no audience, found one in JWT
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImF1ZCI6ICJmcmVkIn0.F-9m2Tx8r1tuQFirazsI4FK05bXX3uP4ut8M2FryJ07k3bQhy262fdwNDmuFcGx0NfL-c80agcwGoTzMWXkVEgZ2KTz0QSAdcdGk3ZWtUy-Mj2IilZ1dzkVvW8LsithYFTGcUtkelFDrJwtMQ0Kum7SXJpC_HCBk4PbftY0XD6jRgHLnQdeT9_J11L4sd19vCdpxxxm3_m_yvUV3ZynzB4vhQbS3CET4EClAVhi-m_gMh9mj85gY1ycIz6-FxWv8xM2Igm2SMeIdyJwAvEGnIauRS928P_OqVCZgCH2Pafnxtzy77Llpxy8XS0xu5PtPw3_azhg33GaXDCFsfz6GpA",
			fmt.Sprintf(`{"cert": "%s"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-wrong-aud", // constraint requires an audience, found wrong one in JWT
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImF1ZCI6ICJmcmVkIn0.F-9m2Tx8r1tuQFirazsI4FK05bXX3uP4ut8M2FryJ07k3bQhy262fdwNDmuFcGx0NfL-c80agcwGoTzMWXkVEgZ2KTz0QSAdcdGk3ZWtUy-Mj2IilZ1dzkVvW8LsithYFTGcUtkelFDrJwtMQ0Kum7SXJpC_HCBk4PbftY0XD6jRgHLnQdeT9_J11L4sd19vCdpxxxm3_m_yvUV3ZynzB4vhQbS3CET4EClAVhi-m_gMh9mj85gY1ycIz6-FxWv8xM2Igm2SMeIdyJwAvEGnIauRS928P_OqVCZgCH2Pafnxtzy77Llpxy8XS0xu5PtPw3_azhg33GaXDCFsfz6GpA",
			fmt.Sprintf(`{"cert": "%s", "aud": "cath"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
		{
			"rs256-wrong-aud-list", // constraint requires an audience, found list of wrong ones in JWT
			"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAieHh4IiwgImF1ZCI6IFsiZnJlZCIsICJib2IiXX0.k8jW7PUiMkQCKCjnSFBFFKPDO0RXwZgVkLUwUfi8sMdrrcKi12LC8wd5fLBn0YraFtMXWKdMweKf9ZC-K33h5TK7kkTVKOXctF50mleMlUn0Up_XjtdP1v-2WOfivUXcexN1o-hu0kH7sSQnielXIjC2EAleG6A54YUOZFBdzvd1PKHlsxA7x2iiL73uGeFlyxoaMki8E5tx7FY6JGF1RdhWCoIV5A5J8QnwI5EetduJQ505U65Pk7UApWYWu4l2DT7KCCJa5dJaBvCBemVxWaBhCQWtJKU2ZgOEkpiK7b_HsdeRBmpG9Oi1o5mt5ybC09VxSD-lEda_iJO_7i042A",
			fmt.Sprintf(`{"cert": "%s", "aud": "cath"}`, certPemPs),
			false,
			`{}`,
			`{}`,
			"",
		},
	}

	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	tests := []test{}

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`[%#v, %s, %s]`, p.valid, p.header, p.payload)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		tests = append(tests, test{
			p.note,
			[]string{fmt.Sprintf(`p = [x, y, z] { time.now_ns(now); io.jwt.decode_verify("%s", %s, [x, y, z]) }`, p.token, p.constraints)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range tests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}
}

func TestTopDownJWTEncodeSign(t *testing.T) {

	astHeaderHS256Term := ast.MustParseTerm(`{"typ": "JWT", "alg": "HS256"}`)
	astPayloadTerm := ast.MustParseTerm(`{"iss": "joe", "exp": 1300819380, "aud": ["bob", "saul"], "http://example.com/is_root": true, "privateParams": {"private_one": "one", "private_two": "two"}}`)
	astSymmetricKeyTerm := ast.MustParseTerm(`{"kty": "oct", "k": "AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ-EstJQLr_T-1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow"}`)

	astHeaderHS256Obj := astHeaderHS256Term.Value.(ast.Object)
	astPayloadObj := astPayloadTerm.Value.(ast.Object)
	astSymmetricKeyObj := astSymmetricKeyTerm.Value.(ast.Object)

	astHeaderRS256Term := ast.MustParseTerm(`{"alg": "RS256"}`)
	astHeaderRS256Obj := astHeaderRS256Term.Value.(ast.Object)

	astRSAKeyTerm := ast.MustParseTerm(`{"kty": "RSA", "n": "ofgWCuLjybRlzo0tZWJjNiuSfb4p4fAkd_wWJcyQoTbji9k0l8W26mPddxHmfHQp-Vaw-4qPCJrcS2mJPMEzP1Pt0Bm4d4QlL-yRT-SFd2lZS-pCgNMsD1W_YpRPEwOWvG6b32690r2jZ47soMZo9wGzjb_7OMg0LOL-bSf63kpaSHSXndS5z5rexMdbBYUsLA9e-KXBdQOS-UTo7WTBEMa2R2CapHg665xsmtdVMTBQY4uDZlxvb3qCo5ZwKh9kG4LT6_I5IhlJH7aGhyxXFvUK-DWNmoudF8NAco9_h9iaGNj8q2ethFkMLs91kzk2PAcDTW9gb54h4FRWyuXpoQ", "e": "AQAB", "d": "Eq5xpGnNCivDflJsRQBXHx1hdR1k6Ulwe2JZD50LpXyWPEAeP88vLNO97IjlA7_GQ5sLKMgvfTeXZx9SE-7YwVol2NXOoAJe46sui395IW_GO-pWJ1O0BkTGoVEn2bKVRUCgu-GjBVaYLU6f3l9kJfFNS3E0QbVdxzubSu3Mkqzjkn439X0M_V51gfpRLI9JYanrC4D4qAdGcopV_0ZHHzQlBjudU2QvXt4ehNYTCBr6XCLQUShb1juUO1ZdiYoFaFQT5Tw8bGUl_x_jTj3ccPDVZFD9pIuhLhBOneufuBiB4cS98l2SR_RQyGWSeWjnczT0QU91p1DhOVRuOopznQ", "p": "4BzEEOtIpmVdVEZNCqS7baC4crd0pqnRH_5IB3jw3bcxGn6QLvnEtfdUdiYrqBdss1l58BQ3KhooKeQTa9AB0Hw_Py5PJdTJNPY8cQn7ouZ2KKDcmnPGBY5t7yLc1QlQ5xHdwW1VhvKn-nXqhJTBgIPgtldC-KDV5z-y2XDwGUc", "q": "uQPEfgmVtjL0Uyyx88GZFF1fOunH3-7cepKmtH4pxhtCoHqpWmT8YAmZxaewHgHAjLYsp1ZSe7zFYHj7C6ul7TjeLQeZD_YwD66t62wDmpe_HlB-TnBA-njbglfIsRLtXlnDzQkv5dTltRJ11BKBBypeeF6689rjcJIDEz9RWdc", "dp": "BwKfV3Akq5_MFZDFZCnW-wzl-CCo83WoZvnLQwCTeDv8uzluRSnm71I3QCLdhrqE2e9YkxvuxdBfpT_PI7Yz-FOKnu1R6HsJeDCjn12Sk3vmAktV2zb34MCdy7cpdTh_YVr7tss2u6vneTwrA86rZtu5Mbr1C1XsmvkxHQAdYo0", "dq": "h_96-mK1R_7glhsum81dZxjTnYynPbZpHziZjeeHcXYsXaaMwkOlODsWa7I9xXDoRwbKgB719rrmI2oKr6N3Do9U0ajaHF-NKJnwgjMd2w9cjz3_-kyNlxAr2v4IKhGNpmM5iIgOS1VZnOZ68m6_pbLBSp3nssTdlqvd0tIiTHU", "qi": "IYd7DHOhrWvxkwPQsRM2tOgrjbcrfvtQJipd-DlcxyVuuM9sQLdgjVk2oy26F0EmpScGLq2MowX7fhd_QJQ3ydy5cY7YIBi87w93IKLEdfnbJtoOPLUW0ITrJReOgo1cq9SbsxYawBgfp_gh6A5603k2-ZQwVK0JKSHuLFkuQ3U"}`)

	astRSAKeyObj := astRSAKeyTerm.Value.(ast.Object)

	params := []struct {
		note   string
		input1 ast.Object
		input2 ast.Object
		input3 ast.Object
		result string
		err    string
	}{
		{
			"https://tools.ietf.org/html/rfc7515#appendix-A.1",
			astHeaderHS256Obj,
			astPayloadObj,
			astSymmetricKeyObj,

			`"eyJhbGciOiAiSFMyNTYiLCAidHlwIjogIkpXVCJ9.eyJhdWQiOiBbImJvYiIsICJzYXVsIl0sICJleHAiOiAxMzAwODE5MzgwLCAiaHR0cDovL2V4YW1wbGUuY29tL2lzX3Jvb3QiOiB0cnVlLCAiaXNzIjogImpvZSIsICJwcml2YXRlUGFyYW1zIjogeyJwcml2YXRlX29uZSI6ICJvbmUiLCAicHJpdmF0ZV90d28iOiAidHdvIn19.M10TcaFADr_JYAx7qJ71wktdyuN4IAnhWvVbgrZ5j_4"`,
			"",
		},
		{
			"Empty JSON payload",
			astHeaderHS256Obj,
			ast.NewObject(),
			astSymmetricKeyObj,

			`"eyJhbGciOiAiSFMyNTYiLCAidHlwIjogIkpXVCJ9.e30.Odp4A0Fj6NoKsV4Gyoy1NAmSs6KVZiC15S9VRGZyR20"`,
			"",
		},
		{
			"https://tools.ietf.org/html/rfc7515#appendix-A.2",
			astHeaderRS256Obj,
			astPayloadObj,
			astRSAKeyObj,

			`"eyJhbGciOiAiUlMyNTYifQ.eyJhdWQiOiBbImJvYiIsICJzYXVsIl0sICJleHAiOiAxMzAwODE5MzgwLCAiaHR0cDovL2V4YW1wbGUuY29tL2lzX3Jvb3QiOiB0cnVlLCAiaXNzIjogImpvZSIsICJwcml2YXRlUGFyYW1zIjogeyJwcml2YXRlX29uZSI6ICJvbmUiLCAicHJpdmF0ZV90d28iOiAidHdvIn19.ITpfhDICCeVV__1nHRN2CvUFni0yyYESvhNlt4ET0yiySMzJ5iySGynrsM3kgzAv7mVmx5uEtSCs_xPHyLVfVnADKmDFtkZfuvJ8jHfcOe8TUqR1f7j1Zf_kDkdqJAsuGuqkJoFJ3S_gxWcZNwtDXV56O3k_7Mq03Ixuuxtip2oF0X3fB7QtUzjzB8mWPTJDFG2TtLLOYCcobPHmn36aAgesHMzJZj8U8sRLmqPXsIc-Lo_btt8gIUc9zZSgRiy7NOSHxw5mYcIMlKl93qvLXu7AaAcVLvzlIOCGWEnFpGGcRFgSOLnShQX6hDylWavKLQG-VOUJKmtXH99KBK-OYQ"`,
			"",
		},
	}
	type test struct {
		note     string
		rules    []string
		expected interface{}
	}
	var tests []test

	for _, p := range params {
		var exp interface{}
		exp = fmt.Sprintf(`%s`, p.result)
		if p.err != "" {
			exp = errors.New(p.err)
		}

		tests = append(tests, test{
			p.note,
			[]string{fmt.Sprintf(`p = x { io.jwt.encode_sign(%v, %v, %v, x) }`, p.input1, p.input2, p.input3)},
			exp,
		})
	}

	data := loadSmallTestData()

	for _, tc := range tests {
		runTopDownTestCase(t, data, tc.note, tc.rules, tc.expected)
	}

}
