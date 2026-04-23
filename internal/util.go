package internal

import (
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
)

// TODO: add function to ensure podman is installed (and in PATH)

var Hostnames = []string{
	"akarso",
	"arafel",
	"bashar",
	"burseg",
	"caid",
	"caladan",
	"choam",
	"cymek",
	"ecaz",
	"fedaykin",
	"fogwood",
	"frigate",
	"futar",
	"galach",
	"guild",
	"holtzman",
	"hypnobong",
	"inkvine",
	"ixian",
	"kaitain",
	"kanly",
	"krimskell",
	"landsraad",
	"laza",
	"levenbrech",
	"mahdi",
	"melange",
	"mentat",
	"muadru",
	"ocb",
	"phibian",
	"plasteel",
	"plaz",
	"probe",
	"qanat",
	"rachag",
	"rossak",
	"salusan",
	"sandworm",
	"sapho",
	"sardaukar",
	"shigawire",
	"slig",
	"solari",
	"suboid",
	"tleilaxu",
	"umma",
	"verite",
	"windtrap",
	"yali",
}

func RandomHostname() string {
	return Hostnames[rand.IntN(len(Hostnames))]
}

func CopyDir(src, dst string, exclude []string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		for _, pattern := range exclude {
			matched, _ := filepath.Match(pattern, rel)
			if matched {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		return CopyFile(path, target)
	})
}

func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err = os.WriteFile(dst, data, 0755); err != nil {
		return err
	}

	return nil
}
