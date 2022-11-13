package main


import (
	"fmt"
	"os"
	sio "silk/io"
	"silk/kv"
	"silk/net"
	"silk/ui"
	"strings"
)


// ----------------------------------------------------------------------------


func kvUsage() {
	fmt.Printf(`
Usage: %s kv <route>                                     (1)
Usage: %s kv <route> <keys...>                           (2)
Usage: %s kv <route> <key>=<value>...                    (3)
Usage: %s kv --delete <route> <keys...>                  (4)

List (1), get (2), set (3) or delete (4) pairs of <key> and <value> on the
remote servers reached by the given <route>.
These pairs can be used in text substitutions in various other subcommands.

The text substitution works with printf like format strings but instead of
formatting character codes, a key name between curly braces indicates how to
substitute. For exmaple:

    Transferring %{file}: state %{percent}%%

Here the "file" and "percent" would be interpreted as keys and would be
replaced by their values.

`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])

	os.Exit(0)
}


// ----------------------------------------------------------------------------


type kvConfig struct {
	log sio.Logger
	route string
	delete ui.OptionBool
	args []string
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func doKvList(config *kvConfig) {
	var replies []*kv.Reply
	var request kv.Request
	var reply *kv.Reply
	var i, j int

	request.List = true

	replies = doKvRequest(&request, config)

	for i, reply = range replies {
		if i > 0 {
			fmt.Printf("---\n")
		}

		for j = range reply.List {
			fmt.Printf("%s\n", reply.List[j].String())
		}
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func doKvGet(config *kvConfig) {
	var replies []*kv.Reply
	var request kv.Request
	var reply *kv.Reply
	var key kv.Key
	var arg string
	var err error
	var i, j int

	for _, arg = range config.args {
		key, err = kv.NewKey(arg)
		if err != nil {
			fatale(err)
		}

		request.Get = append(request.Get, key)
	}

	replies = doKvRequest(&request, config)

	for i, reply = range replies {
		if i > 0 {
			fmt.Printf("---\n")
		}

		if len(reply.Get) == 1 {
			if reply.Get[0] != kv.NoValue {
				fmt.Printf("%s\n", reply.Get[0].String())
			}
		} else {
			for j = range reply.Get {
				if reply.Get[j] == kv.NoValue {
					fmt.Printf("%s has no value\n",
						request.Get[j].String())
				} else {
					fmt.Printf("%s = %s\n",
						request.Get[j].String(),
						reply.Get[j].String())
				}
			}
		}
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func doKvSet(config *kvConfig) {
	var val, other kv.Value
	var request kv.Request
	var key kv.Key
	var arg string
	var found bool
	var index int
	var err error

	request.Set = make(map[kv.Key]kv.Value)

	for _, arg = range config.args {
		index = strings.Index(arg, "=")
		if index == -1 {
			fatal("not a set expression: '%s'", arg)
		}

		key, err = kv.NewKey(arg[:index])
		if err != nil {
			fatale(err)
		}

		val, err = kv.NewValue(arg[index+1:])
		if err != nil {
			fatale(err)
		}

		other, found = request.Set[key]
		if found {
			fatal("key '%s' assigned twice ('%s' and '%s')",
				key.String(), other.String(), val.String())
		}

		request.Set[key] = val
	}

	doKvRequest(&request, config)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func doKvDelete(config *kvConfig) {
	var request kv.Request
	var arg string
	var key kv.Key
	var err error

	if len(config.args) == 0 {
		fatal("nothing to delete")
	}

	request.Set = make(map[kv.Key]kv.Value)

	for _, arg = range config.args {
		key, err = kv.NewKey(arg)
		if err != nil {
			fatale(err)
		}

		request.Set[key] = kv.NoValue
	}

	doKvRequest(&request, config)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func doKv(config *kvConfig) {
	var arg string

	if config.delete.Value() {
		doKvDelete(config)
	} else if len(config.args) == 0 {
		doKvList(config)
	} else {
		for _, arg = range config.args {
			if strings.Contains(arg, "=") {
				doKvSet(config)
				return
			}
		}
		doKvGet(config)
	}
}

func doKvRequest(request *kv.Request, config *kvConfig) []*kv.Reply {
	var replyc chan *kv.Reply
	var resolver net.Resolver
	var conn net.Connection
	var replies []*kv.Reply
	var route net.Route
	var nreply int

	resolver = net.NewGroupResolver(net.NewAggregatedTcpResolverWith(
		protocol, &net.TcpResolverOptions{
			Log: config.log.WithLocalContext("resolve"),
		}))
	route = net.NewRoute([]string{ config.route }, resolver)

	config.log.Trace("send request (get: %d, list: %t, set: %d)",
		len(request.Get), request.List, len(request.Set))

	route.Send() <- net.MessageProtocol{
		M: request,
		P: protocol,
	}

	close(route.Send())

	replyc = make(chan *kv.Reply)
	nreply = 0

	for conn = range route.Accept() {
		nreply += 1

		go func (conn net.Connection) {
			replyc <- doKvReply(conn)
		}(conn)
	}

	replies = make([]*kv.Reply, 0, nreply)

	for nreply > 0 {
		replies = append(replies, <-replyc)
		nreply -= 1
	}

	close(replyc)

	return replies
}

func doKvReply(conn net.Connection) *kv.Reply {
	var msg net.Message
	var reply *kv.Reply
	var ok bool

	msg, ok = <-conn.RecvN(kv.Protocol, 1)

	close(conn.Send())

	if ok == false {
		return &kv.Reply{}
	}

	reply, ok = msg.(*kv.Reply)

	if ok == false {
		return &kv.Reply{}
	}

	return reply
}


// ----------------------------------------------------------------------------


func kvMain(cli ui.Cli, verbose *verbosity) {
	var config kvConfig = kvConfig{
		delete: ui.OptBool{}.New(),
	}
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { kvUsage() ; return nil },
	}.New()
	var err error
	var ok bool

	cli.AddOption('d', "delete", config.delete)
	cli.DelOption('h', "help")
	cli.AddOption('h', "help", helpOption)

	err = cli.Parse()
	if err != nil {
		fatale(err)
	}

	config.route, ok = cli.SkipWord()
	if ok == false {
		fatal("missing route operand")
	}

	config.args = cli.Arguments()[cli.Parsed():]

	config.log = verbose.log()

	doKv(&config)
}
