# Goblet: Git caching proxy

Goblet is a Git proxy server that caches repositories for read access. Git
clients can configure their repositories to use this as an HTTP proxy server,
and this proxy server serves git-fetch requests if it can be served from the
local cache.

In the Git protocol, the server creates a pack-file dynamically based on the
objects that the clients have. Because of this, caching Git protocol response
is hard as different client needs a different response. Goblet parses the
content of the HTTP POST requests and tells if the request can be served from
the local cache.

This was developed by Google to reduce the automation traffic to googlesource.com. 
Goblet would be useful if you need to run a Git read-only mirroring server to offload
the traffic.

We took the initial implementation from 
[`github.com/google/goblet`](https://github.com/google/goblet) at commit 
`140dd10abcdde487161f1e3c2c6e9b4868f7326c` and added the following modifications:
- Removed all code specific to `googlesource.com`.
- Removed all the GCP-related code and dependencies.
- Added support for caching GitHub repositories, and automatically fetch every 15 minutes.
- Added support for GitHub Apps as authentication mechanism.
- Added DataDog for exporting metrics.
- Removed Bazel; the project now builds with plain Go tooling and a `Makefile`.
- Renamed the module path to `github.com/GetStream/goblet`.
- Bumped to Go 1.26; the Dockerfile build stage uses `golang:1.26-alpine`.
- Migrated metrics from OpenCensus to OpenTelemetry (OTLP exporter — point it
  at the local Datadog Agent's OTLP endpoint).
- Replaced abandoned dependencies: `gopkg.in/square/go-jose.v2` →
  `github.com/go-jose/go-jose/v4`, `github.com/ReneKroon/ttlcache/v2` →
  `github.com/jellydator/ttlcache/v3`, `github.com/libgit2/git2go/v33` → `v34`.

## Usage
1. Build Goblet (requires CGO + libgit2 1.5 — see `install_git2go.sh`)
    ```bash
    make build
    # or: go build -o bin/goblet-server ./goblet-server
    ```
2. Start the Goblet server
    ```bash
    export GH_APP_ID="<APP_ID>"
    export GH_APP_INSTALLATION_ID="<APP_INSTALLATION_ID>"
    export GH_APP_PRIVATE_KEY="<APP_PRIVATE_KEY_PEM_TEXT>"
    ./bin/goblet-server -config "<PATH_TO_CONFIG_FILE>"
    ```

    See `example_config.json` for how the minimum config file should look like.

3. Configure your `Git` client to use `Goblet` as a read-only proxy:
    ```bash
    git config http.proxy "http://localhost:8080"
    git remote set-url origin "http://github.com/<owner>/<repo-name>.git"
    git remote set-url --push origin "git@github.com:<owner>/<repo-name>.git"
    ```
4. Try a `git fetch` command and watch `Goblet`'s outputs to see if it's working 
   as expected.
    ```bash
   git fetch origin master
    ```

## Limitations

Note that Goblet forwards the ls-refs traffic to the upstream server. If the
upstream server is down, Goblet is effectively down. Technically, we can modify
Goblet to serve even if the upstream is down, but the current implementation
doesn't do such thing.
