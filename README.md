Welcome to the open source release version 1.1.1 of ONF's SD-RAN project. Please note that this is a read only release of the source code. We will not be accepting pull requests in these repos, and the source code that is contained here cannot be used to build functional binaries because it refers back to a number of private repositories. But executables - docker containers referencing the released code - are available on Docker Hub and referenced [here](https://wiki.opennetworking.org/display/COM/SD-RAN+1.1+Release).  SD-RAN is currently a member-only project, and ONF membership is required to access the most current release, the master branch, and to do development with SD-RAN.

# RAN Simulator

This software allows simulation of a number of RAN CU/DU nodes and RU cells via the O-RAN E2AP standard.
The simulated RAN environment is described using a YAML model file loaded at start-up.
The simulator offers a gRPC API that can be used at run-time to induce changes in order to 
simulate a dynamically changing environment.

The main RAN simulator software is accompanied by a number of utilities that allow generation of YAML files
that describe large RAN topologies and various environmental metrics, e.g. PCI.

CLI for the RAN simulator is available via `onos-cli ransim` usage and allows access to all major features of
the simulator gRPC API, for controlling and monitoring the changes to the simulated environment.

* You can find all the documentation under [docs](docs)
* See [README.md](docs/README.md) for details of running the RAN simulator application.
* The documentation is also published on [sdran-docs](https://docs.sd-ran.org/master/index.html)
