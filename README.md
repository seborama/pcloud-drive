# pCloud Drive

A client app to mount a pCloud drive on Linux and FreeBSD, for the rest of us who have been forgotten...

It uses FUSE to mount the pCloud drive. This is possible thanks to [Bazil](https://github.com/bazil) and his [FUSE library for Go](https://github.com/bazil/fuse).

I am developing on a Linux ARM Raspberry Pi4. I haven't (yet) tried Linux x86_64 or FreeBSD, it simply is too early at this stage of the development to worry about more than one platform. It should work though.

## Status

At this stage, this is exploratory. The code base is experimental, many features are not implemented or only partially or not accurately.

No write operations are supported for now.

## Getting started

Download the binary for your platform from the releases.

The drive can be mounted via the CLI:

```bash
# Remember to first export the CLI's PCLOUD_* environemt variables!
# export PCLOUD_USERNAME=xxx
# export PCLOUD_PASSWORD=xxx
# export PCLOUD_OTP_CODE=xxx
# replace <mount-point> with a directory that already exists.
pcloud-drive --mount-point <mount-point>

# when you're done:
# replace <mount-point> with a directory that already exists.
umount <mount-point>
```

Should the client end abruptly, or time out, run `umount <mount-point>` to clean up the mount.

## Tests

The tests rely on the presence of environment variables to supply your credentials (**make sure you `export` the variables!**):
- `GO_PCLOUD_USERNAME`
- `GO_PCLOUD_PASSWORD`
- `GO_PCLOUD_TFA_CODE`

**Note**

The device is automatically marked as trusted so TFA is only required the first time, until the trust expires. You can remove the trust manually in your [account security settings](https://my.pcloud.com/#page=settings&settings=tab-security).

TFA was possible thanks to [Glib Dzevo](https://github.com/gdzevo) and his [console-client PR](https://github.com/pcloudcom/console-client/pull/94) where I found the info I needed!

```bash
cd fuse
go test -v ./

mkdir /tmp/pcloud_mnt

# in a separate terminal window:
ls /tmp/pcloud_mnt
# ...

# when you're done:
umount /tmp/pcloud_mnt
```
