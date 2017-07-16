#!/bin/sh

#define parameters which are passed in.
DATE=`date +%Y-%m-%d`
VERSION=`cat ../CHANGELOG.md | grep Unreleased | awk -F " " '{print $2}'`
#define the template.
cat  << EOF
{
    "package": {
        "name": "terraform-provider-vsphere",
        "repo": "binary",
        "subject": "krzkowalczyk"
    },

    "version": {
        "name": "$VERSION-$TRAVIS_COMMIT",
        "desc": "This is a version $VERSION-$TRAVIS_COMMIT",
        "released": "$DATE",
        "gpgSign": false
    },

    "files":
        [
        {"includePattern": "terraform-provider-vsphere", "uploadPattern": "terraform-provider-vsphere/$VERSION/$TRAVIS_COMMIT/terraform-provider-vsphere"}
        ],
    "publish": true
}
EOF
