all: depend build test

SHELL := /bin/bash
.DEFAULT_GOAL := all
BACKUP=gpbackup
RESTORE=gprestore
DIR_PATH=$(shell dirname `pwd`)

DEST = .

GOFLAGS :=
dependencies :
		go get github.com/jmoiron/sqlx
		go get github.com/lib/pq
		go get github.com/maxbrunsfeld/counterfeiter
		go get github.com/onsi/ginkgo/ginkgo
		go get github.com/onsi/gomega
		go get github.com/pkg/errors
		go get golang.org/x/tools/cmd/goimports
		go get gopkg.in/DATA-DOG/go-sqlmock.v1

format :
		goimports -w .
		go fmt ./...

ginkgo :
		ginkgo -r -randomizeSuites -randomizeAllSpecs 2>&1

test : ginkgo

ci : ginkgo

depend : dependencies

build :
		go build -tags '$(BACKUP)' $(GOFLAGS) -o ../../bin/$(BACKUP)
		go build -tags '$(RESTORE)' $(GOFLAGS) -o ../../bin/$(RESTORE)

build_rhel:
		env GOOS=linux GOARCH=amd64 go build -tags '$(BACKUP)' $(GOFLAGS) -o ../../bin/$(BACKUP)
		env GOOS=linux GOARCH=amd64 go build -tags '$(RESTORE)' $(GOFLAGS) -o ../../bin/$(RESTORE)

build_osx:
		env GOOS=darwin GOARCH=amd64 go build -tags '$(BACKUP)' $(GOFLAGS) -o ../../bin/$(BACKUP)
		env GOOS=darwin GOARCH=amd64 go build -tags '$(RESTORE)' $(GOFLAGS) -o ../../bin/$(RESTORE)

install: all installdirs
		$(INSTALL_PROGRAM) gpbackup$(X) '$(DESTDIR)$(bindir)/gpbackup$(X)'

installdirs:
		$(MKDIR_P) '$(DESTDIR)$(bindir)'

clean :
		rm -f $(BACKUP)
		rm -rf /tmp/go-build*
		rm -rf /tmp/ginkgo*
