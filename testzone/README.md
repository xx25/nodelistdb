# The test zone moved

The FidoNet test zone now lives at `/home/dp/FIDO/test-zone`, as its own
project. It is no longer specific to nodelistdb: it is an isolated FTN zone
for testing any FidoNet software against real implementations, and MBSE is
just its first node.

To exercise this repo's test daemon against it:

```bash
cd /home/dp/FIDO/test-zone
make up
python3 bin/run-tests.py --node mbse \
    --daemon /home/dp/src/nodelistdb/bin/testdaemon \
    --config /path/to/testdaemon.yaml
```

The caller's config needs `protocols.telnet.enabled: true` to exercise ITN.

Why it moved: the zone judges a session by the *answering* node's log rather
than the caller's result, so it is useful to anything that speaks FTN, and
keeping it inside one caller's repository made it look like that caller's
test fixture. See `/home/dp/FIDO/test-zone/README.md`.
