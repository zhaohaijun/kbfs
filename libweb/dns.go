package libweb

import (
	"net"
	"strings"
)

const (
	keybaseWebPrefix     = "keybase-web="
	keybaseWebAuthPrefix = "keybase-web-auth="
	sha512Prefix         = "sha512:"
)

type accessControl struct {
	open           bool
	username       string
	passwordSHA512 string
}

func parseAuth(v string) (
	kbfsPath string, parsedAccessControl accessControl, valid bool) {
	fields := strings.Fields(v)
	switch len(fields) {
	case 2:
		switch strings.ToLower(fields[1]) {
		case "open":
			return fields[0], accessControl{open: true}, true
		default:
			return "", accessControl{}, false
		}
	case 3:
		pass := strings.ToLower(fields[2])
		switch {
		case strings.HasPrefix(pass, sha512Prefix):
			return fields[0], accessControl{
				open:           false,
				username:       fields[1],
				passwordSHA512: pass[len(sha512Prefix):],
			}, true
		}
	default:
		return "", accessControl{}, false
	}
}

// dnsConfig is parsed configs from DNS. Valid examples of dns records for
// configuration:
//
// Example 1:
//
// song.gao.io TXT keybase-web=/keybase/public/songgao/web/
//
//
// Example 2:
//
// song.gao.io TXT "keybase-web=/keybase/private/songgao,kb_bot/"
//
// song.gao.io TXT "keybase-web-auth=/keybase/private/songgao,kb_bot/ open"
//
// song.gao.io TXT "keybase-web-auth=/keybase/private/songgao,kb_bot/secret-plan-2016 user sha512:b109f3bbbc244eb82441917ed06d618b9008dd09b3befd1b5e07394c706a8bb980b1d7785e5976ec049b46df5f1326af5a2ea6d103fd07c95385ffab0cacbc86"
//
//
// NOTE: a TXT record that's long than 256 characters can optionally be
// split to multiple quoted strings. This is supported by Go now:
// https://github.com/golang/go/issues/10482
type dnsConfig struct {
	rootPath string
	auth     map[string]accessControl
}

type ErrKeybaseWebRecordNotFound struct{}

func (ErrKeybaseWebRecordNotFound) Error() string {
	return "TXT record not found: " + keybaseWebPrefix
}

// TODO: cache
func loadConfigFromDNS(
	domain string) (config *dnsConfig, err error) {
	txtRecords, err := net.LookupTXT(domain)
	if err != nil {
		return "", err
	}

	config = &dnsConfig{}
	for _, r := range txtRecords {
		r = strings.TrimSpace(r)

		if strings.HasPrefix(r, keybaseWebPrefix) {
			config.rootPath = r[len(keybaseWebPrefix):]
		}

		if strings.HasPrefix(r, keybaseWebAuth) {
			kbfsPath, parsedAccessControl, valid := parseAuth(
				r[len(keybaseWebAuthPrefix):])
			if valid {
				if config.auth == nil {
					config.auth = make(map[string][2]string)
				}
				config.auth[kbfsPath] = parsedAccessControl
			}
		}

	}

	if len(config.rootPath) == 0 {
		return nil, ErrKeybaseWebRecordNotFound{}
	}

	return config, nil
}
