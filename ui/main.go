package main


import (
	sio "silk/io"
	"silk/net"
	"silk/run"
	"time"
)


var mainProtocol net.Protocol = net.NewUint8Protocol(map[uint8]net.Message{
	0: &net.RoutingMessage{},
	1: &run.Message{},
})


func serve(addr string, log sio.Logger) {
	var server net.Accepter = net.NewTcpServer(addr)
	var service run.Service
	var conn net.Connection
	var err error

	service, err = run.NewServiceWith(&run.ServiceOptions{
		Log: log.WithLocalContext("run"),
	})
	if err != nil {
		panic(err.Error())
	}

	for conn = range server.Accept() {
		log.Info("new connection")

		go func (conn net.Connection) {
			var msg net.Message
			var m *run.Message
			var ok bool

			msg = <-conn.RecvN(mainProtocol, 1)
			if msg == nil {
				log.Warn("connection closed unexpectedly")
				close(conn.Send())
				return
			}

			log.Info("receive request")

			m, ok = msg.(*run.Message)
			if ok == false {
				log.Warn("unexpected message type: %T", msg)
				close(conn.Send())
				return
			}

			service.Handle(m, conn)
		}(conn)
	}
}


func main() {
	var log sio.Logger = sio.NewStderrLogger(sio.LOG_TRACE)
	var resolver net.Resolver
	var route net.Route
	var agent run.Agent
	var job run.Job

	go serve(":3200", log.WithGlobalContext("s:3200"))
	go serve(":3201", log.WithGlobalContext("s:3201"))

	log = log.WithGlobalContext("client")

	resolver = net.NewGroupResolver(net.NewTcpResolver(mainProtocol))
	route = net.NewRoute([]string{ "localhost:3200+localhost:3201" },
		resolver)
	job = run.NewJobWith("cat", []string{ "-A" }, route, mainProtocol,
		&run.JobOptions{
			Log: log.WithLocalContext("job"),
		})

	for agent = range job.Accept() {
		log.Info("agent: %s", log.Emph(0, agent.Name()))

		close(agent.Stdin())

		go func (agent run.Agent) {
			var b []byte

			for b = range agent.Stdout() {
				log.Info("%s >> %s", agent.Name(), string(b))
			}

			<-agent.Wait()

			log.Info("%s => %d", agent.Name(), agent.Exit())
		}(agent)
	}

	job.Stdin() <- []byte("Hello World!\n")
	close(job.Stdin())
	<-job.Wait()

	time.Sleep(10 * time.Millisecond)
}
