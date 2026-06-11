// Command server is the read-only data backend for the iPad server dashboard.
// It reads Docker (socket), k3s (kubeconfig) and host metrics (/proc, /sys) and
// serves them as JSON for the frontend to poll.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/magmc/server-projects/dashboard-api/internal/api"
	"github.com/magmc/server-projects/dashboard-api/internal/collect"
	"github.com/magmc/server-projects/dashboard-api/internal/config"
)

func main() {
	cfg := config.Load()

	// Host CPU% needs two samples over time; sample in the background.
	cpu := collect.NewCPUSampler(cfg.HostProc)
	go cpu.Run(2 * time.Second)

	host := collect.NewHostCollector(cfg, cpu)

	// Docker and k3s clients may fail to construct (socket/kubeconfig missing);
	// keep serving the rest and report per-endpoint.
	docker, dockerErr := collect.NewDockerCollector()
	if dockerErr != nil {
		log.Printf("docker collector unavailable: %v", dockerErr)
	}
	k3s, k3sErr := collect.NewK3sCollector(cfg)
	if k3sErr != nil {
		log.Printf("k3s collector unavailable: %v", k3sErr)
	}

	srv := api.NewServer(host, docker, dockerErr, k3s, k3sErr)

	addr := ":" + cfg.Port
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("dashboard-api listening on %s", addr)
	log.Fatal(httpSrv.ListenAndServe())
}
