package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"

	"gopkg.in/square/go-jose.v2"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: certbot-to-pem path-to-certbot-private_key.json")
		os.Exit(1)
	}

	pkBuf, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	var k jose.JSONWebKey
	if err := k.UnmarshalJSON(pkBuf); err != nil {
		panic(err)
	}

	switch p := k.Key.(type) {
	case *rsa.PrivateKey:
		fmt.Println(string(pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(p),
		})))
	default:
		panic("Don't know how to deal with " + reflect.TypeOf(p).String())
	}
}
