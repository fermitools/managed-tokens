Name:           managed-tokens
Version:        0.16.2
Release:        1%{?dist}
Summary:        Utility to obtain Hashicorp vault (service) tokens from service kerberos principals and distribute them to experiment nodes

Group:          Applications/System
License:        Fermitools Software Legal Information (Modified BSD License)
URL:            TODO
Source0:        %{name}-%{version}.tar.gz

%global debug_package %{nil}

BuildRoot:      %(mktemp -ud %{_tmppath}/%{name}-%{version}-XXXXXX)
BuildArch:      x86_64

Requires:       condor
Requires:       condor-credmon-vault
Requires:       jsonnet
Requires:       krb5-workstation
Requires:       iputils
Requires:       rsync
Requires:       sqlite

%description
Utility to obtain Hashicorp vault (service) tokens from service kerberos principals and distribute them to experiment nodes

%prep
test ! -d %{buildroot} || {
rm -rf %{buildroot}
}

%setup -q

%build

%install

# JSONNET/Libsonnet Config files, to /etc/managed-tokens
mkdir -p %{buildroot}/%{_sysconfdir}/%{name}
mkdir -p %{buildroot}/%{_sysconfdir}/%{name}/libsonnet

# libsonnet files
for file in `find libsonnet/ -name '*.libsonnet' -type f`; do
    install -m 0774 ${file} %{buildroot}/%{_sysconfdir}/%{name}/${file}  # Will be something like /etc/managed-tokens/libsonnet/myfile.libsonnet
done

# Main jsonnet file
install -m 0774 managedTokens.jsonnet %{buildroot}/%{_sysconfdir}/%{name}/managedTokens.jsonnet
# Makefile_jsonnet will be installed to /etc/managed-tokens/Makefile
install -m 0774 Makefile_jsonnet %{buildroot}/%{_sysconfdir}/%{name}/Makefile

###
# Executables to /usr/bin
mkdir -p %{buildroot}/%{_bindir}
install -m 0755 refresh-uids-from-ferry %{buildroot}/%{_bindir}/refresh-uids-from-ferry
install -m 0755 token-push %{buildroot}/%{_bindir}/token-push

# Cron and logrotate
mkdir -p %{buildroot}/%{_sysconfdir}/cron.d
install -m 0644 %{name}.cron %{buildroot}/%{_sysconfdir}/cron.d/%{name}
mkdir -p %{buildroot}/%{_sysconfdir}/logrotate.d
install -m 0644 %{name}.logrotate %{buildroot}/%{_sysconfdir}/logrotate.d/%{name}


%clean
rm -rf %{buildroot}

%files
%defattr(0755, rexbatch, fife, 0774)
%{_sysconfdir}/%{name}
%{_sysconfdir}/%{name}/libsonnet
%config(noreplace) %{_sysconfdir}/%{name}/managedTokens.jsonnet
%config(noreplace) %{_sysconfdir}/%{name}/Makefile
%config(noreplace) %{_sysconfdir}/%{name}/libsonnet/*.libsonnet
%config(noreplace) %attr(0644, root, root) %{_sysconfdir}/cron.d/%{name}
%config(noreplace) %attr(0644, root, root) %{_sysconfdir}/logrotate.d/%{name}
%{_bindir}/refresh-uids-from-ferry
%{_bindir}/token-push

%post
# Set owner of /etc/managed-tokens and /etc/managed-tokens/libsonnet
test -d %{_sysconfdir}/%{name} && {
chown rexbatch:fife %{_sysconfdir}/%{name}
}
test -d %{_sysconfdir}/%{name}/libsonnet && {
chown rexbatch:fife %{_sysconfdir}/%{name}/libsonnet
}

# Logfiles at /var/log/managed-tokens
test -d /var/log/%{name} || {
install -d /var/log/%{name} -m 0774 -o rexbatch -g fife
}

# SQLite Database folder at /var/lib/managed-tokens
test -d %{_sharedstatedir}/%{name} || {
install -d %{_sharedstatedir}/%{name} -m 0774 -o rexbatch -g fife
}

# Directory for service-credd vault tokens at /var/lib/managed-tokens/service-credd-vault-tokens
test -d %{_sharedstatedir}/%{name}/service-credd-vault-tokens  || {
install -d %{_sharedstatedir}/%{name}/service-credd-vault-tokens -m 0774 -o rexbatch -g fife
}

%changelog
* Wed Aug 13 2025 Shreyas Bhat <sbhat@fnal.gov> - 0.17
- Added jsonnet dependency
- Removed /etc/managed-tokens/managedTokens.yml from spec
- Added /etc/managed-tokens/managedTokens.jsonnet, /etc/managed-tokens/Makefile, and /etc/managed-tokens/libsonnet/ to spec

* Fri Dec 13 2024 Shreyas Bhat <sbhat@fnal.gov> - 0.16
- Removed run-onboarding-managed-tokens executable from RPM

* Thu Oct 26 2023 Shreyas Bhat <sbhat@fnal.gov> - 0.12
- Added debug_package nil directive
- Added dist macro to RPM release definition

* Thu Oct 26 2023 Shreyas Bhat <sbhat@fnal.gov> - 0.11
- Added directory for service-credd vault tokens at /var/lib/managed-tokens/service-credd-vault-tokens

* Mon Aug 14 2023 Shreyas Bhat <sbhat@fnal.gov> - 0.8
- Added condor_credmon_vault as dependency

* Thu Jun 08 2023 Shreyas Bhat <sbhat@fnal.gov> - 0.8
- Remove templates from spec file - they are now being embedded in the binaries

* Wed Sep 07 2022 Shreyas Bhat <sbhat@fnal.gov> - 0.2.1
- Change owner of /etc/managed-tokens dir

* Mon Aug 29 2022 Shreyas Bhat <sbhat@fnal.gov> - 0.1.0
- First version of the managed tokens RPM
