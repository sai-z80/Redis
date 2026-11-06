# Reference redis-server 7.2.17 (not committed)

The hidden suite was checked against a real `redis-server` 7.2.17. The binary and the release tarball are **not** in this
repository. To rebuild them:

```sh
cd experiment/reference
curl -LO https://download.redis.io/releases/redis-7.2.17.tar.gz
echo "c627dc8da3ded4d68279b1832733e45e3d8a8d1b2f6d24c55ffd2251e48a57b3  redis-7.2.17.tar.gz" | sha256sum -c -
mkdir -p src bin && tar -xzf redis-7.2.17.tar.gz -C src --strip-components=1
```

The pinned commit is `83937c51afb53e1c69661c8e1533e0e19b896fe1`. You can also take the source from that commit on GitHub
(`redis/redis`).

## Build

**With `make`** (simplest): `make -C src MALLOC=libc redis-server && cp src/src/redis-server bin/`

**Without `make`:** this is how the pilot's reference binary was built. `build_redis.sh` is a manual gcc build. Run it with
the source root as the working directory. It writes to `/out/redis-server` and uses `/tmp/rb` as scratch space, so run it
in a container or sandbox where `/out` is a writable mount of `experiment/reference/bin`.

Either build should report `redis-server --version` → `v=7.2.17`. `harness.validate_reference` refuses any other version.
The pilot's binary (libc malloc) had SHA-256 `559705dce766662ad4b0f2aa89fcd970ca1ef7675be2941f1bea8f2bcf46ca92`. Builds are
not reproducible, so a rebuilt binary will usually have a different hash. `freeze --verify` does not check the binary hash.

Then, from `experiment/`: `python3 -m harness.validate_reference --repeats 3`.
