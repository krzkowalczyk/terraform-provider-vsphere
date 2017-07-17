#!/bin/sh

#define parameters which are passed in.
DATE=`date +%Y-%m-%d`
VERSION=`git describe --always --long`

echo $VERSION
echo $DATE

#define the template.
cat  << EOF
{
    "package": {
        "name": "terraform-provider-vsphere",
        "repo": "binary",
        "subject": "krzkowalczyk"
    },

    "version": {
        "name": "$VERSION",
        "desc": "This is a version $VERSION",
        "released": "$DATE",
        "gpgSign": false
    },

    "files":
        [
        {"includePattern": "terraform-provider-vsphere", "uploadPattern": "terraform-provider-vsphere/$VERSION/terraform-provider-vsphere"}
        ],
    "publish": true
}
EOF
