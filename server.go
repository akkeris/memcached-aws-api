package main

import "github.com/go-martini/martini"
import "github.com/martini-contrib/render"
import "github.com/martini-contrib/binding"
import "github.com/aws/aws-sdk-go/aws"
import "github.com/aws/aws-sdk-go/aws/session"
import "github.com/aws/aws-sdk-go/service/elasticache"
import "log"
import "strconv"
import "net/http"
import "database/sql"
import _ "github.com/lib/pq"
import "os"
import "net"
import "strings"
import "io/ioutil"

type provisionspec struct {
	Plan        string `json:"plan"`
	Billingcode string `json:"billingcode"`
}

type tagspec struct {
	Resource string `json:"resource"`
	Name     string `json:"name"`
	Value    string `json:"value"`
}

type Stat struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ErrorMessage struct {
	Error string `json:"error"`
}

func reportError(r render.Render, e string) {
	log.Println("Error: " + e)
	r.JSON(http.StatusInternalServerError, ErrorMessage{Error:e})
}

var region string
var svc *elasticache.ElastiCache

func getMemcachedUrl(name string) (error, string) {
	eresp, awserr := svc.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{
		CacheClusterId:    aws.String(name),
		MaxRecords:        aws.Int64(20),
		ShowCacheNodeInfo: aws.Bool(true),
	})
	if awserr != nil {
		return awserr, ""
	}
	endpointhost := *eresp.CacheClusters[0].CacheNodes[0].Endpoint.Address
	endpointport := strconv.FormatInt(*eresp.CacheClusters[0].CacheNodes[0].Endpoint.Port, 10)
	return nil, endpointhost + ":" + endpointport
}

func tag(name string, key string, value string) (e error) { 
	_, err := svc.AddTagsToResource(&elasticache.AddTagsToResourceInput{
		ResourceName: aws.String("arn:aws:elasticache:" + region + ":" + os.Getenv("ACCOUNTNUMBER") + ":cluster:" + name),
		Tags: []*elasticache.Tag{
			{
				Key:   aws.String(key),
				Value: aws.String(value),
			},
		},
	})
	return err
}

func provision(db *sql.DB, plan string, billingcode string) (error, string) {
	var name string
	err := db.QueryRow("select name from provision where plan=$1 and claimed='no' and make_date=(select min(make_date) from provision where plan=$1 and claimed='no')", plan).Scan(&name)
	if err != nil {
		return err, ""
	}
	err = tag(name, "billingcode", billingcode)
	if err != nil {
		return err, ""
	}
	_, err = db.Exec("update provision set claimed=$1 where name=$2", "yes", name)
	if err != nil {
		return err, ""
	}
	return getMemcachedUrl(name)
}

func deprovision(db *sql.DB, name string) (error) {
	_, err := svc.DeleteCacheCluster(&elasticache.DeleteCacheClusterInput{
		CacheClusterId: aws.String(name),
	})
	if err != nil {
		return err
	}
	_, err = db.Exec("delete from provision where name=$1", name)
	if err != nil {
		return err
	}
	return nil
}

func flushAll(name string) (r string, e error) {
	err, memcached_url := getMemcachedUrl(name)
	if err != nil {
		return err.Error(), err
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp4", memcached_url)
	if err != nil {
		return err.Error(), err
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return err.Error(), err
	}
	_, err = conn.Write([]byte("flush_all\n"))
	conn.CloseWrite()
	if err != nil {
		return err.Error(), err
	}
	result, err := ioutil.ReadAll(conn)
	if err != nil {
		return err.Error(), err
	}
	trimmed := strings.TrimSpace(string(result))
	return trimmed, nil
}

func getStats(name string) (s []Stat, e error) {
	var stats []Stat
	err, memcached_url := getMemcachedUrl(name)
	if err != nil {
		return stats, err
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp4", memcached_url)
	if err != nil {
		return stats, err
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return stats, err
	}
	_, err = conn.Write([]byte("stats\n"))
	conn.CloseWrite()
	if err != nil {
		return stats, err
	}
	result, err := ioutil.ReadAll(conn)
	if err != nil {
		return stats, err
	}
	resulta := strings.Split(string(result), "\n")
	for _, element := range resulta {
		if strings.HasPrefix(element, "STAT") {
			var stat Stat
			stata := strings.Split(element, " ")
			stat.Key = stata[1]
			t := strings.TrimSpace(stata[2])
			stat.Value = t
			stats = append(stats, stat)
		}
	}
	return stats, nil
}

func main() {
	if os.Getenv("ACCOUNTNUMBER") == "" {
		log.Fatalln("The aws ACCOUNTNUMBER environment variable was not set and is required.")
	}
	if os.Getenv("BROKERDB") == "" {
		log.Fatalln("The postgres broker database is required for this to run as environment BROKERDB in the format postgres://...")
	}
	if os.Getenv("REGION") == "" {
		log.Fatalln("The REGION environment variable is not defined, it should contain the AWS region (e.g., us-west-2).")
	}
	db, dberr := sql.Open("postgres", os.Getenv("BROKERDB"))
	if dberr != nil {
		log.Fatalln(dberr.Error())
		return
	}
	region = os.Getenv("REGION")
	svc = elasticache.New(session.New(&aws.Config{
		Region: aws.String(region),
	}))
	m := martini.Classic()
	m.Use(render.Renderer())

	m.Post("/v1/memcached/instance", binding.Json(provisionspec{}), func (spec provisionspec, berr binding.Errors, r render.Render) {
		if berr != nil {
			r.JSON(400, "Malformed request")
			return
		}
		err, memcached_url := provision(db, spec.Plan, spec.Billingcode)
		if err != nil {
			reportError(r, err.Error())
			return
		}
		r.JSON(http.StatusOK, map[string]string{"MEMCACHED_URL":memcached_url})
	})
	m.Delete("/v1/memcached/instance/:name", func(params martini.Params, r render.Render) {
		err := deprovision(db, params["name"])
		if err != nil {
			reportError(r, err.Error())
			return
		}
		r.JSON(http.StatusOK, map[string]interface{}{"response":"memcached removed"})
	})
	m.Get("/v1/memcached/plans", func(r render.Render) {
		plans := make(map[string]interface{})
		plans["small"] = "Small - 1x CPU - 0.6 GB "
		plans["medium"] = "Medium - 2x CPU - 3.2 GB"
		plans["large"] = "Large - 2x CPU 6 GB"
		r.JSON(http.StatusOK, plans)
	})
	m.Get("/v1/memcached/url/:name", func(params martini.Params, r render.Render) {
		err, memcached_url := getMemcachedUrl(params["name"])
		if err != nil {
			reportError(r, err.Error())
			return
		}
		r.JSON(http.StatusOK, map[string]string{"MEMCACHED_URL":memcached_url})
	})
	m.Post("/v1/tag", binding.Json(tagspec{}), func (spec tagspec, berr binding.Errors, r render.Render) {
		if berr != nil {
			r.JSON(400, "Malformed request")
			return
		}
		err := tag(spec.Resource, spec.Name, spec.Value)
		if err != nil {
			reportError(r, err.Error())
			return
		}
		r.JSON(http.StatusOK, map[string]interface{}{"response": "tag added"})
	})
	m.Get("/v1/memcached/operations/stats/:name", func (params martini.Params, r render.Render) {
		stats, err := getStats(params["name"])
		if err != nil {
			reportError(r, err.Error())
			return
		}
		r.JSON(http.StatusOK, stats)
	})
	m.Delete("/v1/memcached/operations/cache/:name", func (params martini.Params, r render.Render) {
		result, err := flushAll(params["name"])
		if err != nil {
			reportError(r, err.Error())
			return
		}
		r.JSON(http.StatusOK, map[string]string{"flush_all": result})
	})
	m.Run()
}
