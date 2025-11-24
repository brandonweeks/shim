# Minimal Shim

A bazelisk-style launcher for the `minimal` tool that automatically downloads and executes the correct version.

## How It Works

1. Fetches a JSON config from Google Cloud Storage to determine the latest version (commit hash)
2. Checks if that version is already cached in `~/.cache/minimal/`
3. Downloads the binary from private GitHub Releases using authenticated API requests
4. Executes the binary with all command-line arguments forwarded

## Setup

### Building the Shim

```bash
cd shim
go build -o minimal
```

### Configuration

Before building, you need to configure authentication for the private GitHub repository:

1. **Create a GitHub Personal Access Token** with read-only access:
   - Go to GitHub Settings → Developer settings → Personal access tokens → Fine-grained tokens
   - Create a token with "Contents" read-only permission for the `gominimal/minimal` repository
   - Or use a classic token with `repo` scope

2. **Update the token in `main.go`**:
   ```go
   const githubToken = "ghp_YOUR_ACTUAL_TOKEN_HERE"
   ```

3. **Verify the config URL** (already set to GCS bucket):
   ```go
   const configURL = "https://storage.googleapis.com/minimal-shim-config/config2.json"
   ```

**Security Note**: The GitHub token will be embedded in the compiled binary. Only use tokens with minimal required permissions (read-only access to releases). Consider using fine-grained tokens scoped to a single repository.

### JSON Config Format

The JSON config should contain a `version` field with the commit hash:

```json
{
  "version": "abc123def456"
}
```

### Installing

Copy the built `minimal` binary to a directory in your PATH:

```bash
sudo cp minimal /usr/local/bin/minimal
```

## Usage

Once installed, use it like the regular `minimal` command:

```bash
minimal <args>
```

The shim will:
- Download the correct version on first run
- Reuse the cached version on subsequent runs
- Automatically update when you change the version in the JSON config

## Cache Location

Binaries are cached in:
```
~/.cache/minimal/minimal-<commit-hash>
```

## GitHub Release Requirements

The shim uses the GitHub API to download release assets from the private `gominimal/minimal` repository.

**Release Asset Naming**: Binaries must be named:
```
minimal-<commit-hash>
```

For example, for version `abc123def`, the release tag and asset should be:
- **Release tag**: `abc123def`
- **Asset name**: `minimal-abc123def`

The shim will:
1. Query the GitHub API for release with tag matching the version in config.json
2. Find the asset with name `minimal-<version>`
3. Download it using authenticated request

## Architecture Support

Currently supports Linux amd64 only. To add support for other platforms, modify the `downloadBinary` function to detect `runtime.GOOS` and `runtime.GOARCH` and construct the appropriate download URL.
