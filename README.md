# pCloud Drive

A client app to mount a pCloud drive on Linux, for the rest of us who have been forgotten...

It uses FUSE to mount the pCloud drive. This is possible thanks to [Bazil](https://github.com/bazil) and his [FUSE library for Go](https://github.com/bazil/fuse).

pCloud integration is leveraged from [seborama/pcloud-sdk](https://github.com/seborama/pcloud-sdk)

I am developing on a Linux ARM Raspberry Pi4. I haven't (yet) tried Linux x86_64, it is too early at this stage of the development to worry about more than one platform. It should work the same though.

## Status

The drive is theoretically fully functional, read and write. Attributes are also supported although, of course, pCloud applies its own cloud model for ownership and permissions.

What this lacks is sufficient hindsight and use to flesh out bugs and performance issues.

This means that **`read`** should be considered **BETA** and **`write`** should be considered **EXPERIMENTAL**.

## Getting started

Download the binary for your platform from the releases, if available, or build it yourself.

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

The device is automatically marked as trusted by pCloud, so TFA is only required the first time, until the trust expires. You can remove the trust manually in your [account security settings](https://my.pcloud.com/#page=settings&settings=tab-security).

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
