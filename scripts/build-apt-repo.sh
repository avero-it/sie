#!/usr/bin/env bash
# Build a signed apt repository from a directory of .deb files.
#
#   APT_GPG_PRIVATE_KEY=<armored key> scripts/build-apt-repo.sh <debs-dir> <out-dir>
#
# Needs apt-ftparchive (package apt-utils) and gpg. The result in <out-dir> is
# what gets served from GitHub Pages: pool/, dists/stable/ (Release, InRelease,
# Release.gpg, Packages per architecture), the public key (sie.gpg binary,
# sie.asc armored) and a small index.html. See .github/workflows/apt.yml.
set -euo pipefail

debs=${1:?usage: build-apt-repo.sh <debs-dir> <out-dir>}
out=${2:?usage: build-apt-repo.sh <debs-dir> <out-dir>}
: "${APT_GPG_PRIVATE_KEY:?APT_GPG_PRIVATE_KEY is not set}"

archs="amd64 arm64"
base_url="https://avero-it.github.io/sie"

shopt -s nullglob
files=("$debs"/*.deb)
if [ ${#files[@]} -eq 0 ]; then
  echo "no .deb files in $debs" >&2
  exit 1
fi

export GNUPGHOME
GNUPGHOME=$(mktemp -d)
trap 'rm -rf "$GNUPGHOME"' EXIT
chmod 700 "$GNUPGHOME"
printf '%s\n' "$APT_GPG_PRIVATE_KEY" | gpg --batch --quiet --import
fpr=$(gpg --batch --list-secret-keys --with-colons | awk -F: '/^fpr:/ {print $10; exit}')
[ -n "$fpr" ] || { echo "no secret key imported" >&2; exit 1; }

rm -rf "$out"
mkdir -p "$out/pool/main/s/sie"
cp "${files[@]}" "$out/pool/main/s/sie/"

cd "$out"
for arch in $archs; do
  dir="dists/stable/main/binary-$arch"
  mkdir -p "$dir"
  apt-ftparchive --arch "$arch" packages pool > "$dir/Packages"
  gzip -9n -k -f "$dir/Packages"
done

apt-ftparchive \
  -o APT::FTPArchive::Release::Origin=sie \
  -o APT::FTPArchive::Release::Label=sie \
  -o APT::FTPArchive::Release::Suite=stable \
  -o APT::FTPArchive::Release::Codename=stable \
  -o APT::FTPArchive::Release::Architectures="$archs" \
  -o APT::FTPArchive::Release::Components=main \
  release dists/stable > dists/stable/Release

gpg --batch --yes --pinentry-mode loopback --default-key "$fpr" \
  --clearsign -o dists/stable/InRelease dists/stable/Release
gpg --batch --yes --pinentry-mode loopback --default-key "$fpr" \
  --armor --detach-sign -o dists/stable/Release.gpg dists/stable/Release

gpg --batch --export "$fpr" > sie.gpg
gpg --batch --armor --export "$fpr" > sie.asc

echo "built apt repository in $out with $(ls pool/main/s/sie | wc -l) package file(s), key $fpr"
