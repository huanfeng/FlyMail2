package imap

import (
	"fmt"

	"github.com/emersion/go-sasl"
)

// xoauth2Client implements the XOAUTH2 SASL mechanism (used by Gmail / Outlook).
type xoauth2Client struct {
	username string
	token    string
}

func newXOAuth2Client(username, token string) sasl.Client {
	return &xoauth2Client{username: username, token: token}
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	mech = "XOAUTH2"
	ir = []byte(fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", c.username, c.token))
	return mech, ir, nil
}

func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	// Server returned an error challenge (JSON). Send empty response to terminate.
	return []byte{}, nil
}
