package main

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
	"os"
	"time"
	"flag"
	"sort"
)

var queryName string
var queryTags StringSliceFlag
var serviceName string

func init() {
	queryTags = StringSliceFlag{}
	queryTags.Set("master")
	flag.StringVar(&queryName, "consul-query-name", "", "-consul-query-name master")
	flag.Var(&queryTags, "consul-query-tag", "-consul-query-tag serviceName (tag 'master' is set by default)")
	flag.StringVar(&serviceName, "consul-service-name", "", "-consul-service-name serviceName")
	flag.Parse()

	sort.Strings(queryTags)

	if queryName == "" {
		log.Fatal("argument -consul-query-name is not set")
	}

	if serviceName == "" {
		log.Fatal("argument -consul-service-name is not set")
	}
}

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
		defer func(lock *api.Lock) {
			if r := recover(); r != nil {
				lock.Unlock()
				panic(r)
			}
		} (lockCheck)
		// retry to lock
		if lockCheckHeld == false {
			continue
		}
		// try to get master node
		queryResponse, _, err := getMaster(client)
		if err != nil {
			panic(err)
		}
		// check if there are any master nodes
		if len(queryResponse.Nodes) == 0 {
			// try to get master state
			lockHeld, lock := consulLock(client, fmt.Sprintf("master-%s-%s", queryName, serviceName), 0*time.Second)
			defer func(lock *api.Lock) {
				if r := recover(); r != nil {
					lock.Unlock()
					panic(r)
				}
			} (lock)
			if lockHeld {
				// set master state
				updateTag(client, "master")
				// unlock 'master' lock
				lock.Unlock()
				break
			} else {
				// unlock check lock
				lockCheck.Unlock()
				time.Sleep(1 * time.Second)
				continue
			}
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
			// sort queries service tags to be able to compare them to the given query tags
			sort.Strings(query.Service.Tags)
			// compare tags
			tagsEqual := slicesEqual(query.Service.Tags, queryTags)
			// compare service names
			serviceEqual := (query.Service.Service == serviceName)
			// check if wee need to recreate the query
			recreateQuery := !tagsEqual || !serviceEqual
			if recreateQuery {
				log.Printf("deleting existing query '%s' with wrong configuration", query.Name)
				_, err := preparedQuery.Delete(query.ID, &api.QueryOptions{})
				if err != nil {
					panic(err)
				}
				break
			}
			log.Printf("found query: %s", query.Name)
			masterQuery = *query
			break
		}
	}
	// check if query exists
	if masterQuery.ID == "" {
		// create query
		log.Printf("query not found, creating query '%s'", queryName)

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

func slicesEqual(a, b []string) bool {

	if a == nil && b == nil {
		return true;
	}

	if a == nil || b == nil {
		return false;
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
