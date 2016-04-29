// Copyright (c) 2016, Stephen Gallagher <sgallagh@redhat.com>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice,
//    this list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from this
//    software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

// VersionMajor sscg major version
const VersionMajor int = 1

// VersionMinor sscg minor version
const VersionMinor int = 0

// VersionPatch sscg patch version
const VersionPatch int = 0

var debugIO = ioutil.Discard
var verboseIO = ioutil.Discard
var standardIO = ioutil.Discard

// DebugLogger Debug printouts
var DebugLogger *log.Logger

// VerboseLogger Logger for progress
var VerboseLogger *log.Logger

// StandardLogger Display basic output
var StandardLogger *log.Logger

func parseArgs(sc *SscgConfig) error {
	var err error

	// Store the current working directory to use in defaults
	sc.cwd, err = os.Getwd()
	if err != nil {
		return err
	}

	// --quiet
	flag.BoolVar(&sc.quiet, "quiet", false, "Display no output unless there is an error.")

	// --verbose
	flag.BoolVar(&sc.verbose, "verbose", false, "Display progress messages.")

	// --debug
	flag.BoolVar(&sc.debug, "debug", false, "Enable logging of debug messages. Implies verbose.\n\tWarning! This will print private key information to the screen!")

	// --version
	flag.BoolVar(&sc.printVersion, "version", false, "Display the version number and exit")

	// --lifetime
	flag.UintVar(&sc.lifetime, "lifetime", 3650, "Certificate lifetime (days).\n\t")

	// --key-strength
	sc.keyStrength = KeyStrength2048
	flag.Var(&sc.keyStrength, "key-strength", "Strength of the certificate private keys in bits. {512,1024,2048,4096}\n\t")

	// --hash-alg
	sc.hashAlgorithm = HashAlgorithmSHA256
	flag.Var(&sc.hashAlgorithm, "hash-alg", "Hashing algorithm to use for signing. {sha256,sha384,sha512}\n\t")

	// --package
	flag.StringVar(&sc.packagename, "package", "Unknown", "The name of the package needing a certificate\n\t")

	// --ca-file
	sc.caFile = fmt.Sprintf("%s/ca.crt", sc.cwd)
	flag.StringVar(&sc.caFile, "ca-file", sc.caFile, "Path where the public CA certificate will be stored.\n\t")

	// --cert-file
	sc.certFile = fmt.Sprintf("%s/service.pem", sc.cwd)
	flag.StringVar(&sc.certFile, "cert-file", sc.certFile, "Path where the public service certificate will be stored.\n\t")

	// --cert-key-file
	sc.certKeyFile = fmt.Sprintf("%s/service-key.pem", sc.cwd)
	flag.StringVar(&sc.certKeyFile, "cert-key-file", sc.certKeyFile, "Path where the public service certificate will be stored.\n\t")

	// --hostname
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	flag.StringVar(&sc.hostname, "hostname", hostname, "The valid hostname of the certificate. Must be an FQDN.\n\t")

	// --subject-alt-name
	flag.Var(&sc.subjectAltNames, "subject-alt-name", "An additional valid hostname for the certificate. May be specified multiple times.\n\t")

	// --country
	flag.StringVar(&sc.country, "country", "US", "Certificate DN: Country (C)\n\t")

	// --organization
	flag.StringVar(&sc.organization, "organization", "Unspecified", "Certificate DN: Organization (O)\n\t")

	flag.Parse()

	return nil
}

func main() {
	var sc = new(SscgConfig)

	err := parseArgs(sc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parsing arguments failed: %s\n", err)
		os.Exit(1)
	}

	if sc.printVersion {
		fmt.Printf("%d.%d.%d\n", VersionMajor, VersionMinor, VersionPatch)
	}

	if !sc.quiet {
		standardIO = os.Stdout
	}
	StandardLogger = log.New(standardIO, "[OUTPUT] ", 0)

	if sc.verbose || sc.debug {
		verboseIO = os.Stdout
	}
	VerboseLogger = log.New(verboseIO, "", log.Ldate|log.Ltime|log.Lmicroseconds)

	if sc.debug {
		debugIO = os.Stdout
	}
	DebugLogger = log.New(debugIO, "[DEBUG] ", 0)

	DebugLogger.Printf("%+#v\n", sc)

	// Compare the cert-file and cert-key-file arguments
	// We don't want to write anything out if this comparison fails

	matched, err := sc.SamePath(sc.certFile, sc.certKeyFile)
	if err != nil {
		os.Exit(1)
	}

	// Create a private CA to sign the certificate
	VerboseLogger.Printf("Generating private CA")
	err = sc.createPrivateCA()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Creating private CA failed: %s\n", err)
		os.Exit(1)
	}
	VerboseLogger.Printf("Private CA generated successfully\n")

	if sc.debug {
		sc.CertificateDebug()
	}

	// Create a service certificate and sign it with the private CA
	err = sc.createServiceCert()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Creating service certificate failed: %s\n", err)
		os.Exit(1)
	}
	VerboseLogger.Printf("Service certificate generated successfully\n")

	if sc.debug {
		sc.CertificateDebug()
	}

	/* == Write the output files == */
	var data []byte

	// Write the CA public certificate

	if data, err = sc.caCertificate.MarshalPEM(); err != nil {
		fmt.Fprintf(os.Stderr, "Error getting CA certificate data: %v\n", err)
		os.Exit(1)
	}

	err = sc.WriteSecureFile(sc.caFile, data)
	if err != nil {
		os.Exit(1)
	}
	StandardLogger.Printf("CA public certificate written to %s.\n", sc.caFile)

	// Write the public service certificate

	if data, err = sc.svcCertificate.MarshalPEM(); err != nil {
		fmt.Fprintf(os.Stderr, "Error getting service certificate data: %v\n", err)
		os.Exit(1)
	}

	err = sc.WriteSecureFile(sc.certFile, data)
	if err != nil {
		os.Exit(1)
	}
	StandardLogger.Printf("Service public certificate written to %s.\n", sc.certFile)

	// Write the service private key
	if data, err = sc.svcCertificateKey.MarshalPKCS1PrivateKeyPEM(); err != nil {
		fmt.Fprintf(os.Stderr, "Error getting service certificate data: %v\n", err)
		os.Exit(1)
	}

	if matched {
		// If they are the same file, append to it
		err = sc.AppendToFile(sc.certKeyFile, data)
		if err != nil {
			os.Exit(1)
		}
	} else {
		// Otherwise, create it as normal
		err = sc.WriteSecureFile(sc.certKeyFile, data)
		if err != nil {
			os.Exit(1)
		}
	}
	StandardLogger.Printf("Service certificate private key written to %s.\n", sc.certKeyFile)
}