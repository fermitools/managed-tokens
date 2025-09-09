NAME = managed-tokens
VERSION = v0.17.0
ROOTDIR = $(shell pwd)
BUILD = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
rpmVersion := $(subst v,,$(VERSION))
buildTarName = $(NAME)-$(rpmVersion)
buildTarPath = $(ROOTDIR)/$(buildTarName).tar.gz
SOURCEDIR = $(ROOTDIR)/$(buildTarName)
executables = refresh-uids-from-ferry token-push
libsonnetDir = $(ROOTDIR)/libsonnet
libsonnetFiles = $(shell find $(libsonnetDir) -name '*.libsonnet' -maxdepth 1 -type f)
specfile := $(ROOTDIR)/packaging/$(NAME).spec
ifdef RACE
raceflag := -race
else
raceflag :=
endif
ifdef SIGN
signflagCommit := -S
else
signflagCommit :=
endif


all: spec git-tag build tarball rpm
.PHONY: all spec git-tag build tarball rpm git-revert-tag clean clean-all


spec:
	sed -Ei 's/Version\:[ ]*.+/Version:        $(rpmVersion)/' $(specfile)
	echo "Set version in spec file to $(rpmVersion)"


git-tag: spec
	git add Makefile $(specfile)
	git commit $(signflagCommit) -m 'Release $(VERSION)'
	git tag $(VERSION)


build:
	for exe in $(executables); do \
		echo "Building $$exe"; \
		cd cmd/$$exe;\
		go build $(raceflag) -ldflags="-X main.buildTimestamp=$(BUILD)" -o $(ROOTDIR)/$$exe;  \
		echo "Built $$exe"; \
		cd $(ROOTDIR); \
	done


tarball: build
	mkdir -p $(SOURCEDIR)
	mkdir -p $(SOURCEDIR)/libsonnet
	cp $(foreach exe,$(executables),$(ROOTDIR)/$(exe)) $(SOURCEDIR)  # Executables
	cp $(ROOTDIR)/managedTokens.jsonnet $(ROOTDIR)/Makefile_jsonnet $(ROOTDIR)/packaging/managed-tokens.logrotate $(ROOTDIR)/packaging/managed-tokens.cron $(SOURCEDIR)  # Config files
	cp $(foreach lsfile,$(libsonnetFiles),$(lsfile)) $(SOURCEDIR)/libsonnet/$(notdir $(lsfile)) # Libsonnet files
	tar -czf $(buildTarPath) -C $(ROOTDIR) $(buildTarName)
	echo "Built deployment tarball"


rpm: rpmSourcesDir := $$HOME/rpmbuild/SOURCES
rpm: rpmSpecsDir := $$HOME/rpmbuild/SPECS
rpm: rpmDir := $$HOME/rpmbuild/RPMS/x86_64/
rpm: spec tarball
	cp $(specfile) $(rpmSpecsDir)/
	cp $(buildTarPath) $(rpmSourcesDir)/
	cd $(rpmSpecsDir); \
	rpmbuild -ba ${NAME}.spec
	find $$HOME/rpmbuild/RPMS -type f -name "$(NAME)-$(rpmVersion)*.rpm" -cmin 1 -exec cp {} $(ROOTDIR)/ \;
	echo "Created RPM and copied it to current working directory"


git-revert-tag:
	git tag -d $(VERSION)
	git reset --hard HEAD~1
	echo "Reverted git tag and commit for version $(VERSION)"

clean:
	(test -e $(buildTarPath)) && (rm $(buildTarPath))
	(test -e $(SOURCEDIR)) && (rm -Rf $(SOURCEDIR))


clean-all: clean git-revert-tag
	(test -e $(ROOTDIR)/$(NAME)-$(rpmVersion)*.rpm) && (rm $(ROOTDIR)/$(NAME)-$(rpmVersion)*.rpm)
