set -eu
version=v3.277.0
case "$(uname -m)" in
  x86_64) arch=x64; checksum=1e38f0fb69a14a326cf3acfaf975770e8de5d0771042f26292f0013d237b8393 ;;
  aarch64|arm64) arch=arm64; checksum=b9c2708bd13ffdcba79f555a0532893a9b5a44efca8e225d171b8f7b605b16ce ;;
  *) echo "Unsupported Linux architecture: $(uname -m)" >&2; exit 1 ;;
esac
install_dir="$(mktemp -d "$RUNNER_TEMP/magelift-pulumi.XXXXXX")"
cd "$install_dir"
curl --fail --silent --show-error --location --retry 3 \
  "https://github.com/pulumi/pulumi/releases/download/$version/pulumi-$version-linux-$arch.tar.gz" -o pulumi.tar.gz
printf '%s  pulumi.tar.gz\n' "$checksum" | sha256sum --check -
tar -xzf pulumi.tar.gz pulumi/pulumi
printf '%s\n' "$install_dir/pulumi" >> "$GITHUB_PATH"
