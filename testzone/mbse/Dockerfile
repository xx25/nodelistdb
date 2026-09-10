# MBSE BBS mailer (mbcico), built from source, answering on the three
# internet transports a FidoNet node advertises: IBN (binkp), IFC (ifcico
# over EMSI) and ITN (EMSI over telnet).
#
# The point of this image is interop: everything else in the nodelistdb test
# suite is Go talking to Go, and the bugs this container exists to catch are
# the ones that only appear between implementations -- a ZMODEM batch, a
# telnet option exchange, a binkp end-of-session -- against the C mailer whose
# sysop reported them.
#
# Debian bookworm on purpose: MBSE is old C, and GCC 12 still treats implicit
# declarations as warnings where GCC 14 makes them errors.
FROM debian:bookworm-slim

ARG MBSE_REPO=https://git.code.sf.net/p/mbsebbs/code
# v1.1.7.2 -- the release the reporting sysop runs. Pinned so the image cannot
# drift out from under a test result.
ARG MBSE_COMMIT=b09252f4d61eb3cb8e5c8d561da00925d651a7c1

ENV MBSE_ROOT=/opt/mbse
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential autoconf automake ca-certificates git \
        zlib1g-dev libbz2-dev libncurses-dev \
        socat procps iproute2 net-tools telnet \
    && rm -rf /var/lib/apt/lists/*

# MBSE insists on these accounts: mbtask is setuid root and drops to mbse,
# and the install targets are owned by mbse:bbs.
RUN groupadd bbs \
    && useradd -c "MBSE BBS Admin" -d /opt/mbse -g bbs -m -s /bin/bash mbse \
    && useradd -c "MBSE BBS Login" -d /opt/mbse/home/bbs -g bbs -s /bin/false bbs

RUN git clone "${MBSE_REPO}" /usr/src/mbsebbs \
    && cd /usr/src/mbsebbs \
    && git checkout -q "${MBSE_COMMIT}" \
    && git log --oneline -1

WORKDIR /usr/src/mbsebbs
RUN ./configure && make && make install

# The config patcher links against MBSE's own header, so struct sysconfig can
# never drift between the tool and the binaries reading config.data.
COPY patch-config.c /usr/src/patch-config.c
RUN gcc -o /usr/local/bin/patch-config /usr/src/patch-config.c \
        -I/usr/src/mbsebbs/lib -I/usr/src/mbsebbs \
    && chmod 0755 /usr/local/bin/patch-config

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/entrypoint.sh

# IBN, IFC, ITN on the ports a nodelist entry would advertise. The zone has
# its own network, so nothing has to be remapped and a caller under test can
# use its ordinary per-protocol defaults.
EXPOSE 24554 60179 23

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
