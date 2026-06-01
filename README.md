# ODOL

This repo contains tooling to convert Bohemia Interactive P3D files from ODOLv6 and ODOLv7 to MLOD format.

## Usage

The tool takes an input P3D file and emits an MLOD version and a model.cfg file to match.

```
$ odol-convert
usage: odol-convert <input.p3d>

$ odol-convert m4a1.p3d
convert:        input P3D: m4a1.p3d
convert:      output MLOD: m4a1_mlod.p3d
convert: output model.cfg: m4a1.model.cfg

convert: detected input format: ODOLv6
convert: converting ODOLv6 to MLOD + model.cfg
convert: parsing ODOL data
convert: validating full decode
convert: writing output files
convert: conversion complete

$ ls -lart
-rw-r--r--@   1 jmhobbs  staff   811824 Jun  1 10:58 m4a1.p3d
-rw-r--r--@   1 jmhobbs  staff  5861839 Jun  1 10:58 m4a1_mlod.p3d
-rw-r--r--@   1 jmhobbs  staff      680 Jun  1 10:58 m4a1.model.cfg
```

## Contributing

Find a bug? I'm not surprised. This is clanker code, generated from BI wiki docs and lots of sample files.  File an issue and send me your input file and we will see what can be done about it, but I wouldn't go reading the code if I was you.
