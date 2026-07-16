package main

import (
	"github.com/nodelistdb/internal/version"
	"github.com/xx25/fidomail/pkg/emsi"
)

// stampEMSIIdentity sets the mailer identity advertised in our outbound
// EMSI_DAT. NewSessionWithInfoAndConfig has neutral defaults, so without
// this the mailer name/version fields go out empty. Shared by the
// single-modem (main.go) and multi-modem (worker.go) paths.
func stampEMSIIdentity(cfg *emsi.Config) {
	cfg.MailerName = "NodelistDB"
	cfg.MailerVersion = version.GetVersionInfo()
}
