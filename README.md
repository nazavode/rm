# `rm` - reMarkable cloud tools

## `rmd` - Sync Pocket to reMarkable cloud

`rmd` is a [Pocket](https://getpocket.com) to [reMarkable cloud](https://my.remarkable.com/login) sync daemon. It carries out its job by:

1. retrieving articles saved on Pocket that are marked with a specific tag or starred as favorites;
2. converting them to `EPUB` (via [`pandoc`](https://pandoc.org));
3. uploading them to reMarkable cloud into a specific location so you will be able to read them on your reMarkable tablet.

### Installation

```shell
$ go get github.com/nazavode/rm/cmd/rmd
```

Please note that to be able to convert (sanitized and polished) `HTML` content to `EPUB`, [`pandoc`](https://pandoc.org) must be installed and available in `$PATH`.

### Getting started

In order to be able to authenticate on both ends of the data flow, you should obtain proper
tokens from both the cloud services involved:

1. [Pocket](https://getpocket.com) device and user tokens, set either via command line (`--pocket-token` and `--pocket-user`) or via environment variables (`$RMD_POCKET_TOKEN` and `$RMD_POCKET_KEY`)
2. [reMarkable cloud](https://my.remarkable.com/login) device token, set either via command line (`--rm-device`) or via environment variable (`$RMD_RM_DEVICE_TOKEN`)

With proper authentication tokens in place, the sync daemon can be started with:

```shell
$ rmd
```
