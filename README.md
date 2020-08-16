# Single Page App Server 

Minimal server for serving a single-page app

## Build

Pointing to the root directory, run:

```bash
make
```

The compiled executable can be found in the `build/` directory.
Then, to run it:

```bash
./build/serve
```

## Deployment

### Build for target platform

You can pass in environment variables to compile for a different platform. For example, on a Linux x86 (AWS typically) box:

```bash
env GOOS=linux GOARCH=386 make
```

### Copy to target

```bash
scp -i path/to/downloaded/ec2/pem path/to/build/serve ec2-user@ec2-ip-addr.compute-1.amazonaws.com:/home/ec2-user/targetdirectory
```

### Point to certificates

After using CertBot to generate the `fullchain.pem` and `privkey.pem` files, specify these when running it on the live box:

```bash
sudo ./serve -port 443 -rootdir data_test -ssl -fullchain '/path/to/fullchain.pem' -privkey 'path/to/privkey.pem'
``
