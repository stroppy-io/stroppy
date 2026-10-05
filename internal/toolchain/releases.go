package toolchain

import (
	"fmt"
	"runtime"
)

const downloadBaseURL = "https://go.dev/dl/"

type release struct {
	File   string
	SHA256 string
}

var releases = map[string]release{
	"aix/ppc64": {
		"go1.27.1.aix-ppc64.tar.gz",
		"43b4827f082b94b7b6de28a49dd80df268e06935fd82a1b57aab3a772343bf6c",
	},
	"darwin/amd64": {
		"go1.27.1.darwin-amd64.tar.gz",
		"8f8f52c6649542cf027bbc9b9c68d1ec042f9f34808a40413f0b8b3f66f3caa4",
	},
	"darwin/arm64": {
		"go1.27.1.darwin-arm64.tar.gz",
		"ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12",
	},
	"dragonfly/amd64": {
		"go1.27.1.dragonfly-amd64.tar.gz",
		"bda1f327b9a792be80da856a3ea8f92cbfd138a3991e0a97049ee4d4904196a2",
	},
	"freebsd/386": {
		"go1.27.1.freebsd-386.tar.gz",
		"a01f190a079150ada777fcd1a7fb27ab1a98caf9f09b2411b2ffb7e9486e0030",
	},
	"freebsd/amd64": {
		"go1.27.1.freebsd-amd64.tar.gz",
		"9761194512938a640948ae5ba273df73a441f4229ea21a1503937e55c69bf107",
	},
	"freebsd/arm": {
		"go1.27.1.freebsd-arm.tar.gz",
		"e5b55c9f6646373c221f8384cee60321128e7f48b42da3b7da7bce3fae44a88c",
	},
	"freebsd/arm64": {
		"go1.27.1.freebsd-arm64.tar.gz",
		"8b155a723a584fb29125c6b36fc9cb0d5489a3f7fb76670508a2c51af718074d",
	},
	"illumos/amd64": {
		"go1.27.1.illumos-amd64.tar.gz",
		"ed04719b94b176745e08847f48d8a9e92b49330f2ff4ca2cf34e1b66cb5fd682",
	},
	"linux/386": {
		"go1.27.1.linux-386.tar.gz",
		"3b72028095439d2bc0ce84e271cc70328a878d879020c5721eaa46df5f72fbc0",
	},
	"linux/amd64": {
		"go1.27.1.linux-amd64.tar.gz",
		"63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445",
	},
	"linux/arm64": {
		"go1.27.1.linux-arm64.tar.gz",
		"3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec",
	},
	"linux/arm": {
		"go1.27.1.linux-armv6l.tar.gz",
		"44893f200fb034791d4188df9fc9b9e73eadbb5fceafd5166703f0b9bab73fc2",
	},
	"linux/loong64": {
		"go1.27.1.linux-loong64.tar.gz",
		"a3df3db12e0b16d932bbfd3f6025473faf8bfeb13ea2177de5e8fac1d4124b7f",
	},
	"linux/mips": {
		"go1.27.1.linux-mips.tar.gz",
		"a6e637864e6801408d2d8d7f5d429b90bf8854ac320eb70778f4ac36e9b8e889",
	},
	"linux/mips64": {
		"go1.27.1.linux-mips64.tar.gz",
		"ba3d2d4969d901cdf15d930f0cbcaa761da9d4c98386a6a26d4a82cdf596aa9f",
	},
	"linux/mips64le": {
		"go1.27.1.linux-mips64le.tar.gz",
		"23f9b4ce864764849ca792d8cf6bd8e00880ef3c6070a2717509f8d3f6059ba5",
	},
	"linux/mipsle": {
		"go1.27.1.linux-mipsle.tar.gz",
		"06cfb6c5528a54d53580c6390efea9ad0362322518bc32129ea6c0145f1e6a23",
	},
	"linux/ppc64": {
		"go1.27.1.linux-ppc64.tar.gz",
		"e91dee51bdfa08e034d9ed4016929a2008f179caa004922d637a97d5cda05778",
	},
	"linux/ppc64le": {
		"go1.27.1.linux-ppc64le.tar.gz",
		"ad1ff83c24f783c2d4a025852b928a05c199e8742a14a5482d88de0293d2bd0d",
	},
	"linux/riscv64": {
		"go1.27.1.linux-riscv64.tar.gz",
		"62287667ee5e5f540f30fb9b7529a27fe582f22c6bfd726ece9b045f4c54ee61",
	},
	"linux/s390x": {
		"go1.27.1.linux-s390x.tar.gz",
		"c9e1ad7bddea40e2eb10b4d3267ae06d462667839bf0ba94c2e2d160b06f9b9e",
	},
	"netbsd/386": {
		"go1.27.1.netbsd-386.tar.gz",
		"6bde412641300348cc571cfcf3384071934c5005c8057325e29763963107b932",
	},
	"netbsd/amd64": {
		"go1.27.1.netbsd-amd64.tar.gz",
		"4ace14c14e7c6eb1d06890433afe81f8c2dc746d1d5f4df3ae6d2d7dd0481ba2",
	},
	"netbsd/arm": {
		"go1.27.1.netbsd-arm.tar.gz",
		"da7db485266e309cca263bcad95197d2c21e747e14698bc1d95704e9939fd07a",
	},
	"netbsd/arm64": {
		"go1.27.1.netbsd-arm64.tar.gz",
		"c0ac5527cdde89e31d40e536a1cec4a8d8b2903f79e2356645c862dfab6f3181",
	},
	"openbsd/386": {
		"go1.27.1.openbsd-386.tar.gz",
		"3434ae1ef067666d4df50f883e985a49e7589d6004a412329e7684ff80f40852",
	},
	"openbsd/amd64": {
		"go1.27.1.openbsd-amd64.tar.gz",
		"db0566b3b3ddf6eda09cb3434a9401eb2583ffa539475e45592812813f11c792",
	},
	"openbsd/arm": {
		"go1.27.1.openbsd-arm.tar.gz",
		"5423e99d338cf9ccebbfce0e373095e63b62fbedd7a1b99436bb7e0f3184069c",
	},
	"openbsd/arm64": {
		"go1.27.1.openbsd-arm64.tar.gz",
		"953142ae3734098e65118ddca29ed2f469a85f039a78a1572da98ba271042e65",
	},
	"openbsd/ppc64": {
		"go1.27.1.openbsd-ppc64.tar.gz",
		"93e32d7c0ba24e56f5f75aed454e9301c7204605c11508d65ecb39356c221106",
	},
	"openbsd/riscv64": {
		"go1.27.1.openbsd-riscv64.tar.gz",
		"1683c8e86157739bfee080d38919750366aeed8fe77576b0798460a2f06d6b98",
	},
	"solaris/amd64": {
		"go1.27.1.solaris-amd64.tar.gz",
		"7efd1b6d470ea724db178f17d3bd1b66931e2aa79c4389fe75228f2aef6014a4",
	},
}

func hostRelease() (release, error) {
	target := runtime.GOOS + "/" + runtime.GOARCH

	value, ok := releases[target]
	if !ok {
		return release{}, fmt.Errorf("%w: %s", ErrUnsupportedHost, target)
	}

	return value, nil
}
