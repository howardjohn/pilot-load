# File Based Istio

This tool connects to Pilot, then dumps the XDS responses to files that can be read directly from Envoy.

The config will be slightly modified to change all config sources to point to the relevant files rather than Pilot. The bootstrap is also custom (and static). Aside from this, the config will be the same as from Pilot.

## Install

`go get github.com/howardjohn/file-based-istio`

## Usage

* The `-o` flag should be provided for the output directory, otherwise everything is output to stdout.
* The `-n` flag can be provided to call Pilot as the pod. This is needed to get inbound listener, and maybe some other configs.
* The `-p` flag can be provided to change the Pilot address to use. By default this is `localhost:15010`.

Example: `file-based-istio -o install/files -n 10.28.0.166 -p localhost:15010`

This generates all of the config needed into the install folder, which can be installed with:

`helm template install | kubectl apply -f -`

`replace --force` may be needed instead of `apply` as a hack to not write the config as an annotation, as large EDS responses can exceed the size limit on annotations.
