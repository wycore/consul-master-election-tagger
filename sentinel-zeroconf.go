package main

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
	"os"
	"time"
)

// todo: add as arguments
var queryName string = "sensu-master"
var queryTags []string = []string{"sensu", "master"}
var serviceName string = "redis"

func main() {

	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		panic(err)
	}

	var lockCheck *api.Lock
	var lockCheckHeld bool
	for i := 0; i < 5; i++ {
		log.Printf("%d. try", i+1)
		// lock for tag update to prevent race condition
		lockCheckHeld, lockCheck = consulLock(client, fmt.Sprintf("check-%s-%s", queryName, serviceName), 10*time.Second)
		// retry to lock
		if lockCheckHeld == false {
			continue
		}
		// try to get master node
		queryResponse, _, err := getMaster(client)
		if err != nil {
			lockCheck.Unlock()
			panic(err)
		}
		// check if there are any master nodes
		if len(queryResponse.Nodes) == 0 {
			// try to get master state
			lockHeld, lock := consulLock(client, fmt.Sprintf("master-%s-%s", queryName, serviceName), 0*time.Second)
			if lockHeld {
				// set master state
				updateTag(client, "master")
				break
			} else {
				time.Sleep(1 * time.Second)
				continue
			}
			// unlock 'master' lock
			lock.Unlock()
		} else {
			if agentInQueryResponse(client.Agent(), queryResponse) && len(queryResponse.Nodes) == 1 {
				log.Println("I'm the current master")
				break
			}
			// set slave state
			updateTag(client, "slave")
			break
		}
	}
	lockCheck.Unlock()
	// unlock check lock
	os.Exit(0)
}

func agentInQueryResponse(agent *api.Agent, queryResponse *api.PreparedQueryExecuteResponse) bool {
	nodeName, err := agent.NodeName()
	if err != nil {
		panic(err)
	}
	for _, node := range queryResponse.Nodes {
		if node.Node.Node == nodeName {
			return true
		}
	}

	return false
}

func updateTag(client *api.Client, tag string) {
	// todo: check if tag is already present
	agent := client.Agent()
	services, err := agent.Services()
	if err != nil {
		panic(err)
	}
	service := services[serviceName]

	if inSlice(tag, service.Tags) {
		log.Printf("Tag '%s' already present", tag)
		return
	}

	log.Printf("trying to add tag '%s' to service '%s'", tag, service.Service)

	serviceRegistration := &api.AgentServiceRegistration{
		ID:                service.ID,
		Name:              service.Service,
		Tags:              append(cleanupTagSlice(service.Tags), tag),
		Port:              service.Port,
		Address:           service.Address,
		EnableTagOverride: service.EnableTagOverride,
	}

	err = agent.ServiceRegister(serviceRegistration)
	if err != nil {
		panic(err)
	}

	log.Printf("successfully added tag '%s' to service '%s'", tag, service.Service)
}

func getMaster(client *api.Client) (*api.PreparedQueryExecuteResponse, *api.QueryMeta, error) {
	preparedQuery := client.PreparedQuery()
	preparedQueries, _, err := preparedQuery.List(&api.QueryOptions{})
	if err != nil {
		panic(err)
	}

	var masterQuery api.PreparedQueryDefinition
	for _, query := range preparedQueries {
		if query.Name == queryName {
			log.Printf("found query: %s", query.Name)
			masterQuery = *query
			break
		}
	}
	if masterQuery.ID == "" {
		log.Println("query not found, creating")

		masterQueryDefinition := api.PreparedQueryDefinition{
			Name: queryName,
			Service: api.ServiceQuery{
				Service:     serviceName,
				OnlyPassing: true,
				Tags:        queryTags,
			},
		}
		newMasterQueryId, _, err := preparedQuery.Create(&masterQueryDefinition, &api.WriteOptions{})
		if err != nil {
			panic(err)
		}
		masterQueryDefinition.ID = newMasterQueryId
		masterQuery = masterQueryDefinition
	}

	return preparedQuery.Execute(masterQuery.ID, &api.QueryOptions{})
}

func consulLock(client *api.Client, key string, lockWaitTime time.Duration) (bool, *api.Lock) {

	//kv := client.KV()
	//session := client.Session()

	lock, err := client.LockOpts(&api.LockOptions{Key: key, LockTryOnce: true, LockWaitTime: lockWaitTime})
	if err != nil {
		panic(err)
	}

	lockChan, err := lock.Lock(nil)
	if err != nil {
		panic(err)
	}
	lockHeld := false
	if lockChan == nil {
		log.Printf("lock aquisition for '%s' failed", key)
	} else {
		log.Printf("got lock for '%s'", key)
		lockHeld = true
	}

	return lockHeld, lock
}

// removes the "master" and "slave" from the given slice
func cleanupTagSlice(slice []string) []string {
	var result []string
	for _, v := range slice {
		if v == "master" || v == "slave" {
			continue
		}
		result = append(result, v)
	}

	return result
}

func inSlice(element string, slice []string) bool {
	for _, el := range slice {
		if el == element {
			return true
		}
	}

	return false
}
