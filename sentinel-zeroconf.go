package main

import (
	"github.com/hashicorp/consul/api"
	"log"
	"os"
	"time"
)

func main() {

	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		panic(err)
	}

	for i := 0; i < 5; i++ {
		queryResponse, _, err := getMaster(client)
		if err != nil {
			panic(err)
		}

		if len(queryResponse.Nodes) == 0 {
			lockHeld := consulLock(client)
			if lockHeld {
				updateTag(client, "master")
				break
			} else {
				time.Sleep(1 * time.Second)
				continue
			}
		} else {
			updateTag(client, "slave")
			break
		}
	}

	os.Exit(0)
}

func updateTag(client *api.Client, tag string) {
	agent := client.Agent()
	services, err := agent.Services()
	if err != nil {
		panic(err)
	}
	service := services["redis"]

	serviceRegistration := &api.AgentServiceRegistration{
		ID: service.ID,
		Name: service.Service,
		Tags: []string{"sensu", tag},
		Port: service.Port,
		Address: service.Address,
		EnableTagOverride: service.EnableTagOverride,
	}

	err = agent.ServiceRegister(serviceRegistration)
	if err != nil {
		panic(err)
	}
}

func getMaster(client *api.Client) (*api.PreparedQueryExecuteResponse, *api.QueryMeta, error){
	preparedQuery := client.PreparedQuery()
	preparedQueries, _, err := preparedQuery.List(&api.QueryOptions{})
	if err != nil {
		panic(err)
	}

	var masterQuery api.PreparedQueryDefinition
	for _, query := range preparedQueries {
		if query.Name == "sensu-master" {
			log.Printf("found query: %+v", query)
			masterQuery = *query
			break
		}
	}
	if masterQuery.ID == "" {
		log.Println("query not found, creating")

		masterQueryDefinition := api.PreparedQueryDefinition{Name: "sensu-master", Service: api.ServiceQuery{Service: "redis", OnlyPassing: true, Tags: []string{"sensu", "master"}}}
		newMasterQueryId, _, err := preparedQuery.Create(&masterQueryDefinition, &api.WriteOptions{})
		if err != nil {
			panic(err)
		}
		masterQueryDefinition.ID = newMasterQueryId
		masterQuery = masterQueryDefinition
	}

	return preparedQuery.Execute(masterQuery.ID, &api.QueryOptions{})
}

func consulLock(client *api.Client) bool {

	//kv := client.KV()
	//session := client.Session()

	lock, err := client.LockOpts(&api.LockOptions{Key: "sensu-master", LockTryOnce: true, LockWaitTime: 0 * time.Second})
	if err != nil {
		panic(err)
	}

	lockChan, err := lock.Lock(nil)
	if err != nil {
		panic(err)
	}
	lockHeld := false
	if lockChan == nil {
		log.Println("lock aquisition failed")
	} else {
		log.Println("got lock")
		lockHeld = true
	}

	return lockHeld
}
