# bunny-upload

Bunny CDN storage zone uploader.
That tool uploads directory contents recursively, and (optionally) purges cache

## Configuration

### Config file

```bash
bunny-upload -c /path/to/config.yml
```

[Example config file](./config.yml.sample)

### Arguments

```bash
Usage of bunny-upload:
  -a string
    	access key (pull zone)
  -i int
    	pull zone ID
  -k string
    	access key (storage zone password)
  -p string
    	path to the folder to upload recursively
  -z string
    	storage zone
```
