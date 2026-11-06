#!/bin/sh
# Manual build of redis-server 7.2.17 (no make available). Run inside sandbox, cwd = redis source root.
set -e
B=/tmp/rb; mkdir -p $B/lua $B/deps $B/srv
CF="-O1 -std=c99 -pedantic -w -DREDIS_STATIC= -D_GNU_SOURCE"
# lua
cd deps/lua/src
for f in *.c; do case $f in lua.c|luac.c|print.c) continue;; esac
  gcc -O1 -w -DLUA_ANSI -DENABLE_CJSON_GLOBAL -DREDIS_STATIC= -DLUA_USE_MKSTEMP -c $f -o $B/lua/${f%.c}.o & done; wait
cd ../../..
gcc -O1 -w -c deps/hdr_histogram/hdr_histogram.c -Ideps/hdr_histogram -DHDR_MALLOC_INCLUDE=\"hdr_redis_malloc.h\" -o $B/deps/hdr.o
for f in net hiredis sds async read alloc sockcompat; do gcc -O1 -w -std=c99 -D_GNU_SOURCE -c deps/hiredis/$f.c -o $B/deps/hiredis_$f.o; done
gcc -O1 -w -c deps/fpconv/fpconv_dtoa.c -o $B/deps/fpconv.o
cd src
sh ./mkreleasehdr.sh >/dev/null 2>&1 || true
OBJS="$(sed -n 's/^REDIS_SERVER_OBJ=//p' Makefile)"
for o in $OBJS; do
  gcc $CF -I../deps/hiredis -I../deps/linenoise -I../deps/lua/src -I../deps/hdr_histogram -I../deps/fpconv -DUSE_LIBC_MALLOC -c ${o%.o}.c -o $B/srv/$o &
done; wait
cd $B/srv; gcc -o /out/redis-server *.o $B/lua/*.o $B/deps/*.o -lm -ldl -lpthread -lrt
