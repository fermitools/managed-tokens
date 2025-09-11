FROM opensciencegrid/osg-wn

ARG rpmfile

COPY ${rpmfile} /tmp/${rpmfile}
RUN dnf install -y /tmp/${rpmfile} && dnf clean all

RUN useradd testuser
USER testuser
ENTRYPOINT ["/usr/bin/token-push", "--version"]
