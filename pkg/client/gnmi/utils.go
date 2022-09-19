// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package gnmi

import (
	"context"
	"crypto/tls"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/certs"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	baseClient "github.com/openconfig/gnmi/client"
	gclient "github.com/openconfig/gnmi/client/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"time"

	"google.golang.org/grpc/credentials"
)

const defaultTimeout = 60 * time.Second

func setCertificate(pathCert string, pathKey string) tls.Certificate {
	certificate, err := tls.LoadX509KeyPair(pathCert, pathKey)
	if err != nil {
		log.Error("could not load client key pair ", err)
	}
	return certificate
}

// passCred is an username/password implementation of credentials.Credentials.
type passCred struct {
	username string
	password string
	secure   bool
}

// GetRequestMetadata returns the current request metadata, including
// username and password in this case.
// This implements the required interface fuction of credentials.Credentials.
func (pc *passCred) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"username": pc.username,
		"password": pc.password,
	}, nil
}

// RequireTransportSecurity indicates whether the credentials requires transport security.
// This implements the required interface fuction of credentials.Credentials.
func (pc *passCred) RequireTransportSecurity() bool {
	return pc.secure
}

// newPassCred returns a newly created passCred as credentials.Credentials.
func newPassCred(username, password string, secure bool) credentials.PerRPCCredentials {
	return &passCred{
		username: username,
		password: password,
		secure:   secure,
	}
}

func Connect(ctx context.Context, d baseClient.Destination, opts ...grpc.DialOption) (*client, error) {
	switch d.TLS {
	case nil:
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	default:
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(d.TLS)))
	}

	if d.Credentials != nil {
		secure := true
		if d.TLS == nil {
			secure = false
		}
		pc := newPassCred(d.Credentials.Username, d.Credentials.Password, secure)
		opts = append(opts, grpc.WithPerRPCCredentials(pc))
	}

	gCtx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	addr := ""
	if len(d.Addrs) != 0 {
		addr = d.Addrs[0]
	}
	conn, err := grpc.DialContext(gCtx, addr, opts...)
	if err != nil {
		return nil, errors.NewInternal("Dialer(%s, %v): %v", addr, d.Timeout, err)
	}

	cl, err := gclient.NewFromConn(gCtx, conn, d)
	if err != nil {
		return nil, err
	}

	gnmiClient := &client{
		client: cl,
	}

	return gnmiClient, nil
}

func NewDestination(address string, targetID topoapi.ID, tlsOptions *topoapi.TLSOptions) (*baseClient.Destination, error) {

	timeout := defaultTimeout

	destination := &baseClient.Destination{
		Addrs:   []string{address},
		Target:  string(targetID),
		Timeout: timeout,
	}

	if tlsOptions.Plain {
		log.Info("Plain (non TLS) connection to ", address)
	} else {
		tlsConfig := &tls.Config{}
		if tlsOptions.Insecure {
			log.Info("Insecure TLS connection to ", address)
			tlsConfig = &tls.Config{InsecureSkipVerify: true}
		} else {
			log.Info("Secure TLS connection to ", address)
		}
		if tlsOptions.CaCert == "" {
			log.Info("Loading default CA onfca")
			defaultCertPool, err := certs.GetCertPoolDefault()
			if err != nil {
				return nil, err
			}
			tlsConfig.RootCAs = defaultCertPool
		} else {
			certPool, err := certs.GetCertPool(tlsOptions.CaCert)
			if err != nil {
				return nil, err
			}
			tlsConfig.RootCAs = certPool
		}
		if tlsOptions.Cert == "" && tlsOptions.Key == "" {
			log.Info("Loading default certificates")
			clientCerts, err := tls.X509KeyPair([]byte(certs.DefaultClientCrt), []byte(certs.DefaultClientKey))
			if err != nil {
				return nil, err
			}
			tlsConfig.Certificates = []tls.Certificate{clientCerts}
		} else if tlsOptions.Cert != "" && tlsOptions.Key != "" {
			// Load certs given for device
			tlsConfig.Certificates = []tls.Certificate{setCertificate(tlsOptions.Cert, tlsOptions.Key)}
		} else {
			log.Errorf("Can't load Ca=%s , Cert=%s , key=%s for %v, trying with insecure connection",
				tlsOptions.CaCert, tlsOptions.Cert, tlsOptions.Key, address)
			tlsConfig = &tls.Config{InsecureSkipVerify: true}
		}
		destination.TLS = tlsConfig
	}

	err := destination.Validate()
	if err != nil {
		return nil, err
	}

	return destination, nil
}
