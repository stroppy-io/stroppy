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
		File:   "go1.26.8.aix-ppc64.tar.gz",
		SHA256: "cca7850af0cb7eae901b957b226eaf1306be3cd9b122f362e390ad66178b14e3",
	},
	"darwin/amd64": {
		File:   "go1.26.8.darwin-amd64.tar.gz",
		SHA256: "186be014105aa6542b767d2c6ed5cca10a0214bdff809ef1724022a8c7894150",
	},
	"darwin/arm64": {
		File:   "go1.26.8.darwin-arm64.tar.gz",
		SHA256: "a012b25b571bd0138a03dcd25375ceba866fe5ca822f426d2c66a4de56fd3f4b",
	},
	"dragonfly/amd64": {
		File:   "go1.26.8.dragonfly-amd64.tar.gz",
		SHA256: "0cd8ee4589fd4a5a661826ac46fdf5214e3897e4b36585822425e77e9cee06a9",
	},
	"freebsd/386": {
		File:   "go1.26.8.freebsd-386.tar.gz",
		SHA256: "2a082608129d754d06ad2cb110f3a9d347ba2bf8cac91f587a2a4f21da340729",
	},
	"freebsd/amd64": {
		File:   "go1.26.8.freebsd-amd64.tar.gz",
		SHA256: "458adea37c17ce9a8754a0455fa43e84f27d7e7de3fee81041a13de40c766434",
	},
	"freebsd/arm": {
		File:   "go1.26.8.freebsd-arm.tar.gz",
		SHA256: "60476d7f48f6d79b186f2bcaf2a76fb08a0445b6edad7463f50de8b606cf97cf",
	},
	"freebsd/arm64": {
		File:   "go1.26.8.freebsd-arm64.tar.gz",
		SHA256: "041d822e331b6c45b393bb3314ad890e4f622c4ffb6eba6cd7e934afed4158a9",
	},
	"illumos/amd64": {
		File:   "go1.26.8.illumos-amd64.tar.gz",
		SHA256: "a3ed45dc338a6db5edb554e03a37c06a2063022bb7599de4d2e46bbd5d9d2c72",
	},
	"linux/386": {
		File:   "go1.26.8.linux-386.tar.gz",
		SHA256: "55115677dfcd953fa9d2fe376039125a60ff14fe0e619a2339a364860dda64ef",
	},
	"linux/amd64": {
		File:   "go1.26.8.linux-amd64.tar.gz",
		SHA256: "d0f743b33e8d8945e6b1f432edd15785c70507121d6e2a723b21285eddf8b57b",
	},
	"linux/arm64": {
		File:   "go1.26.8.linux-arm64.tar.gz",
		SHA256: "211ffced9dcb9633a55eac6364816ec0ddd951389a740e88fa8b3337971bdda0",
	},
	"linux/arm": {
		File:   "go1.26.8.linux-armv6l.tar.gz",
		SHA256: "eab440beabf395870752021fa74cedf04f97b61e43ebbe005ecbb98b94e55697",
	},
	"linux/loong64": {
		File:   "go1.26.8.linux-loong64.tar.gz",
		SHA256: "64fb507c00d6f830f71a342feb80b05641b4da5d5ca1d5e06896d5572f8a458c",
	},
	"linux/mips": {
		File:   "go1.26.8.linux-mips.tar.gz",
		SHA256: "134d7d4c83fb16dc25ef7ad283c28d13a018da06531f6894868ade510bd33e62",
	},
	"linux/mips64": {
		File:   "go1.26.8.linux-mips64.tar.gz",
		SHA256: "dada97e3b32b2d6cb5df319d2223dcd28a7dfe1fcf29ca78d364c937acb09246",
	},
	"linux/mips64le": {
		File:   "go1.26.8.linux-mips64le.tar.gz",
		SHA256: "4d67357ce7909cbf28489562d58150162036dfe696b207a9d49f96d7ea99f814",
	},
	"linux/mipsle": {
		File:   "go1.26.8.linux-mipsle.tar.gz",
		SHA256: "8cf4a4e029cee32f54539625f002820ed134ac3030b43483432d8be124d22e14",
	},
	"linux/ppc64": {
		File:   "go1.26.8.linux-ppc64.tar.gz",
		SHA256: "e4cc0ff1eba37e9b228c49fda1e5eca3a0a29edd3ec7631fbcbc73fae79c7155",
	},
	"linux/ppc64le": {
		File:   "go1.26.8.linux-ppc64le.tar.gz",
		SHA256: "0ddf3ecab842013e6bd618602823a0b8158a18d3e9362f2540463ea5aa184975",
	},
	"linux/riscv64": {
		File:   "go1.26.8.linux-riscv64.tar.gz",
		SHA256: "75d368eef1dc44ff9de0c8cac4b37c519e541e6d46c4291c1ff959644a71af32",
	},
	"linux/s390x": {
		File:   "go1.26.8.linux-s390x.tar.gz",
		SHA256: "339d1a09716815f737cb5ddd355e04e339312b449c268cc417118f4b8137ddd1",
	},
	"netbsd/386": {
		File:   "go1.26.8.netbsd-386.tar.gz",
		SHA256: "039a7ea1da0c1b8771cde7edef70fedebadb6fa9bdeae69be15afa47e0ac98c5",
	},
	"netbsd/amd64": {
		File:   "go1.26.8.netbsd-amd64.tar.gz",
		SHA256: "8f30a489d4985cd61a56d1d47326de1682e6668c99a6a904decc974513a8c43c",
	},
	"netbsd/arm": {
		File:   "go1.26.8.netbsd-arm.tar.gz",
		SHA256: "382b36fecb6c4684db8eec888fa079ba6991f27501bcfb5cf3207ca5baccd79a",
	},
	"netbsd/arm64": {
		File:   "go1.26.8.netbsd-arm64.tar.gz",
		SHA256: "5548fac5866d75d2cdaf39ed5fb832d8864e0b3a045c0c1476d8d2c30f484c9a",
	},
	"openbsd/386": {
		File:   "go1.26.8.openbsd-386.tar.gz",
		SHA256: "c71eff4b5a3a037ddbe3b52ed67c774c60be59b1dce7ba6c5fbdfc33a7e7f257",
	},
	"openbsd/amd64": {
		File:   "go1.26.8.openbsd-amd64.tar.gz",
		SHA256: "338be23e51b38df2fba23b6b523b879dc90aa0408080b36cb68afc21169fd0a9",
	},
	"openbsd/arm": {
		File:   "go1.26.8.openbsd-arm.tar.gz",
		SHA256: "e5031bec4507c289a6809bd6d1256a95077c9e980a650fc85e3403d3117df8f4",
	},
	"openbsd/arm64": {
		File:   "go1.26.8.openbsd-arm64.tar.gz",
		SHA256: "2bb1cc5bf6f988dba69facc51e8d5debcc8d019415a90289976d8fbe53653229",
	},
	"openbsd/ppc64": {
		File:   "go1.26.8.openbsd-ppc64.tar.gz",
		SHA256: "01f9060b3d479692c3b9b0f11b655ea701568ddb62af521328683eee79d967e4",
	},
	"openbsd/riscv64": {
		File:   "go1.26.8.openbsd-riscv64.tar.gz",
		SHA256: "018bae3b35a0402b0e7b2b1c050212e47231c0b00aa8f03727bac79905c49ff7",
	},
	"solaris/amd64": {
		File:   "go1.26.8.solaris-amd64.tar.gz",
		SHA256: "519b707164dc01b46f5aad6ecc7eeeb8531fbcab5082fe0e62a8baf09920e2ff",
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
