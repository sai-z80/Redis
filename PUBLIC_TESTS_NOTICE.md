# Public tests notice

The directory `experiment/tests/hidden/` is included in this repository release.

**Effect of publishing:** those tests are now **public**. They are no longer a withheld scoring oracle.

**Allowed reuse:** visible regression checks, demos, and local debugging.

**Not allowed for future scored experiment cells:** using this same suite as the hidden oracle. Create a new withheld suite, validate it against pinned Redis 7.2.17, freeze it, and only then run contestants.

The freeze recorded in `experiment/FREEZE.json` remains the hash of the suite used for the pilot smoke `pilot-a-deepseek-v3.2-r2` (historical record).
