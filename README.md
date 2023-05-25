Silk
====

Send data to many nodes over arbitrary topology.

- Broadcast files to thousands of nodes in no time
- Control large fleets of machine with parallel remote jobs 
- Speedup long distance communication with TCP aggregation

Silk is the ideal tool for quickly deploying and operating a service over a large fleet of geographically distant machines.

Simpler than [Ansible](https://www.ansible.com/) or [Terraform](https://www.terraform.io/), just launch a `silk server` on each node you need to control and immediately start to send files and run remote processes.
Silk is designed to be a tool that integrate into your shell scripts or any other workflow rather than being a full framework.

More powerful than `ssh`, launch the same command -possibly with different arguments- on many remote nodes in a single line.
Silk communicates in a one-to-many fashion with remote nodes and handle all the parallelism and synchronization for you.

More efficient than `scp` or `rsync`, disseminate your files to countless nodes in a peer-to-peer fashion.
Silk let you control the communcation topology between your nodes and let you optimize the routes you want to use to send your files depending on your needs.


Build
-----

Before to build Silk, make sure you have the following dependencies installed on your system.

- go >= 1.14
- makefile

To build silk, run the following command.

    make all

Additionally, you can run the tests.

    make test                           // run all tests
    make unit-test                      // run Go unit tests only
    make validation-test                // run Silk shell tests only

If you encounter some difficulties in either building Silk or finding that some tests fail in either the `master` or `develop` branch, please [submit an issue](https://github.com/Blockchain-Benchmarking/silk/issues/new/choose).


Usage
-----

To launch a Silk server on a remote node.

    silk server                         // run in the foreground
    daemonize /bin/silk server          // run in the background

Note that [`daemonize`](https://github.com/bmc/daemonize) is an external tool. More options can be provided, see `silk server --help` for a complete list.


To run a command on remote nodes.

    // Run uname on the remote node "my-node" listening on port 3200
    silk run 'my-node:3200' uname -a
    
    // Run ls on two remote nodes 'node-0' and 'node-1'
    silk run '(node-0:3200|node-1:7000)' ls

Sometimes, it is useful to run the same command but with different parameters on each node. This can be achieved with the key-value function of Silk.

    // Set the value for the key 'foo' on each node
    silk kv 'node-0:3200' foo='a'
    silk kv 'node-1:7000' foo=18
    
    // Run a command on two nodes with different arguments
    silk run '(node-0:3200|node-1:7000)' echo "my foo is %{foo}"

More documentation on how to run remote commands or how to use key-value function can be found by running `silk run --help` or `silk kv --help`.


To send a file on remote nodes.

    // Create 'remote-parent-dir/local-file-or-dir' in the node 'my-node'
    silk send -t 'remote-parent-dir' 'my-node:3200' 'local-file-or-dir'
    
    // Send many files ('src0', 'src1') to many nodes ('node-0', 'node-1')
    silk send -t 'dest' '(node-0:3200|node-1:7000)' 'src0' 'src1'

Note the syntax with the `-t` option which might be counter intuitive. The `cp`-like syntax has not yet been implemented.

In the previous example, the local sending node was sending each file to every nodes. This can become inefficient when there is a large number of target node. Instead, Silk can send files in a tree topology.

    // Send 'src' to 4 nodes ('b', 'c', 'd' and 'e')
    silk send -t 'dest' '(a:3200,(b:3200|c:3200)|d:3200,(.|e:3200))' 'src'

In this example, the local node sends the file to two nodes 'a' and 'd'. Then the node 'a' forward it to 'b' and 'c' without writing it on its own disk. In the meantime, the node 'd' forward it to 'e' and the wildcard '.' which means the node 'd' itself.

Note that using complex topologies is not limited to the file sending but can also be used with `silk run` and `silk kv` for example. More documentation on how to send files to remote nodes can be found by running `silk send --help`.
