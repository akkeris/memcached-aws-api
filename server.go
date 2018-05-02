package main

import "github.com/go-martini/martini"
import "github.com/martini-contrib/render"
import "github.com/martini-contrib/binding"
import "github.com/aws/aws-sdk-go/aws"
import "github.com/aws/aws-sdk-go/aws/session"
import "github.com/aws/aws-sdk-go/service/elasticache"
import "fmt"
import "strconv"
import "database/sql"
import _ "github.com/lib/pq"
import "os"
import "net"
import "strings"
import "io/ioutil"
import "encoding/json"

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

func tag(spec tagspec, berr binding.Errors, r render.Render) {
	if berr != nil {
		fmt.Println(berr)
		errorout := make(map[string]interface{})
		errorout["error"] = berr
		r.JSON(500, errorout)
		return
	}
	fmt.Println(spec.Resource)
	fmt.Println(spec.Name)
	fmt.Println(spec.Value)
	svc := elasticache.New(session.New(&aws.Config{
		Region: aws.String(os.Getenv("REGION")),
	}))
	region := os.Getenv("REGION")
	accountnumber := os.Getenv("ACCOUNTNUMBER")
	name := spec.Resource

	arnname := "arn:aws:elasticache:" + region + ":" + accountnumber + ":cluster:" + name

	params := &elasticache.AddTagsToResourceInput{
		ResourceName: aws.String(arnname),
		Tags: []*elasticache.Tag{ // Required
			{
				Key:   aws.String(spec.Name),
				Value: aws.String(spec.Value),
			},
		},
	}
	resp, err := svc.AddTagsToResource(params)

	if err != nil {
		fmt.Println(err.Error())
		errorout := make(map[string]interface{})
		errorout["error"] = berr
		r.JSON(500, errorout)
		return
	}

	fmt.Println(resp)
	r.JSON(200, map[string]interface{}{"response": "tag added"})
}

func provision(spec provisionspec, err binding.Errors, r render.Render) {
	plan := spec.Plan
	billingcode := spec.Billingcode

	brokerdb := os.Getenv("BROKERDB")
	fmt.Println(brokerdb)
	uri := brokerdb
	db, dberr := sql.Open("postgres", uri)
	if dberr != nil {
		fmt.Println(dberr)
		toreturn := dberr.Error()
		r.JSON(500, map[string]interface{}{"error": toreturn})
		return
	}
	var name string
	dberr = db.QueryRow("select name from provision where plan='" + plan + "' and claimed='no' and make_date=(select min(make_date) from provision where plan='" + plan + "' and claimed='no')").Scan(&name)

	if dberr != nil {
		fmt.Println(dberr)
		toreturn := dberr.Error()
		r.JSON(500, map[string]interface{}{"error": toreturn})
		return
	}
	fmt.Println(name)

	stmt, dberr := db.Prepare("update provision set claimed=$1 where name=$2")

	if dberr != nil {
		fmt.Println(dberr)
		toreturn := dberr.Error()
		r.JSON(500, map[string]interface{}{"error": toreturn})
		return
	}
	_, dberr = stmt.Exec("yes", name)
	if dberr != nil {
		fmt.Println(dberr)
		toreturn := dberr.Error()
		r.JSON(500, map[string]interface{}{"error": toreturn})
		return
	}

	region := os.Getenv("REGION")
	svc := elasticache.New(session.New(&aws.Config{
		Region: aws.String(region),
	}))
	accountnumber := os.Getenv("ACCOUNTNUMBER")
	arnname := "arn:aws:elasticache:" + region + ":" + accountnumber + ":cluster:" + name

	params := &elasticache.AddTagsToResourceInput{
		ResourceName: aws.String(arnname),
		Tags: []*elasticache.Tag{ // Required
			{
				Key:   aws.String("billingcode"),
				Value: aws.String(billingcode),
			},
		},
	}
	resp, awserr := svc.AddTagsToResource(params)

	if awserr != nil {
		fmt.Println(awserr.Error())
		toreturn := awserr.Error()
		r.JSON(500, map[string]interface{}{"error": toreturn})
		return
		return
	}

	fmt.Println(resp)
	eparams := &elasticache.DescribeCacheClustersInput{
		CacheClusterId:    aws.String(name),
		MaxRecords:        aws.Int64(20),
		ShowCacheNodeInfo: aws.Bool(true),
	}
	eresp, awserr := svc.DescribeCacheClusters(eparams)
	if awserr != nil {
		toreturn := awserr.Error()
		r.JSON(500, map[string]interface{}{"error": toreturn})
		return
	}
	endpointhost := *eresp.CacheClusters[0].CacheNodes[0].Endpoint.Address
	endpointport := *eresp.CacheClusters[0].CacheNodes[0].Endpoint.Port
	stringport := strconv.FormatInt(endpointport, 10)
	r.JSON(200, map[string]string{"MEMCACHED_URL": endpointhost + ":" + stringport})

}

func main() {
	region := os.Getenv("REGION")
	svc := elasticache.New(session.New(&aws.Config{
		Region: aws.String(region),
	}))

	m := martini.Classic()
	m.Use(render.Renderer())

	m.Post("/v1/memcached/instance", binding.Json(provisionspec{}), provision)

	m.Delete("/v1/memcached/instance/:name", func(params martini.Params, r render.Render) {
		name := params["name"]
		dparams := &elasticache.DeleteCacheClusterInput{
			CacheClusterId: aws.String(name), // Required
		}
		dresp, derr := svc.DeleteCacheCluster(dparams)

		if derr != nil {
			fmt.Println(derr.Error())
			errorout := make(map[string]interface{})
			errorout["error"] = derr.Error()
			r.JSON(500, errorout)
			return
		}
		brokerdb := os.Getenv("BROKERDB")
		fmt.Println(brokerdb)
		uri := brokerdb
		db, dberr := sql.Open("postgres", uri)
		if dberr != nil {
			fmt.Println(dberr)
			toreturn := dberr.Error()
			r.JSON(500, map[string]interface{}{"error": toreturn})
			return
		}

		fmt.Println("# Deleting")
		stmt, err := db.Prepare("delete from provision where name=$1")
		if err != nil {
			errorout := make(map[string]interface{})
			errorout["error"] = err.Error()
			r.JSON(500, errorout)
			return
		}
		res, err := stmt.Exec(name)
		if err != nil {
			errorout := make(map[string]interface{})
			errorout["error"] = err.Error()
			r.JSON(500, errorout)
			return
		}
		affect, err := res.RowsAffected()
		if err != nil {
			errorout := make(map[string]interface{})
			errorout["error"] = err.Error()
			r.JSON(500, errorout)
			return
		}
		fmt.Println(affect, "rows changed")
		r.JSON(200, dresp)
	})
	m.Get("/v1/memcached/plans", func(r render.Render) {
		plans := make(map[string]interface{})
		plans["small"] = "Small - 1x CPU - 0.6 GB "
		plans["medium"] = "Medium - 2x CPU - 3.2 GB"
		plans["large"] = "Large - 2x CPU 6 GB"
		r.JSON(200, plans)
	})

	m.Get("/v1/memcached/url/:name", func(params martini.Params, r render.Render) {
		name := params["name"]
		eparams := &elasticache.DescribeCacheClustersInput{
			CacheClusterId:    aws.String(name),
			MaxRecords:        aws.Int64(20),
			ShowCacheNodeInfo: aws.Bool(true),
		}
		resp, err := svc.DescribeCacheClusters(eparams)
		if err != nil {
			toreturn := err.Error()
			r.JSON(200, map[string]interface{}{"error": toreturn})
			return
		}
		endpointhost := *resp.CacheClusters[0].CacheNodes[0].Endpoint.Address
		endpointport := *resp.CacheClusters[0].CacheNodes[0].Endpoint.Port
		stringport := strconv.FormatInt(endpointport, 10)
		r.JSON(200, map[string]string{"MEMCACHED_URL": endpointhost + ":" + stringport})
	})
	m.Post("/v1/tag", binding.Json(tagspec{}), tag)
	m.Get("/v1/memcached/operations/stats/:name", GetStats)
	m.Delete("/v1/memcached/operations/cache/:name", FlushAll)

	m.Run()
}

func GetStats(params martini.Params, r render.Render) {
	stats, err := getstats(params["name"])
	if err != nil {
		fmt.Println(err)
		r.JSON(500, err)
	}
	r.JSON(200, stats)
}
func FlushAll(params martini.Params, r render.Render) {

	result, err := flushall(params["name"])
	if err != nil {
		fmt.Println(err)
		r.JSON(500, map[string]string{"flush_all": err.Error()})
	}
	r.JSON(200, map[string]string{"flush_all": result})
}

func flushall(name string) (r string, e error) {
	svc := elasticache.New(session.New(&aws.Config{
		Region: aws.String(os.Getenv("REGION")),
	}))
	eparams := &elasticache.DescribeCacheClustersInput{
		CacheClusterId:    aws.String(name),
		MaxRecords:        aws.Int64(20),
		ShowCacheNodeInfo: aws.Bool(true),
	}
	resp, err := svc.DescribeCacheClusters(eparams)
	if err != nil {
		fmt.Println(err)
		return err.Error(), err
	}
	endpointhost := *resp.CacheClusters[0].CacheNodes[0].Endpoint.Address
	endpointport := *resp.CacheClusters[0].CacheNodes[0].Endpoint.Port
	stringport := strconv.FormatInt(endpointport, 10)
	fmt.Println(endpointhost + ":" + stringport)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", endpointhost+":"+stringport)
	if err != nil {
		fmt.Println(err)
		return err.Error(), err
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Println(err)
		return err.Error(), err
	}
	_, err = conn.Write([]byte("flush_all\n"))
	conn.CloseWrite()
	if err != nil {
		fmt.Println(err)
		return err.Error(), err
	}
	result, err := ioutil.ReadAll(conn)
	fmt.Println(string(result))
	if err != nil {
		fmt.Println(err)
		return err.Error(), err
	}
	trimmed := strings.TrimSpace(string(result))
	return trimmed, nil

}
func getstats(name string) (s []Stat, e error) {
	var stats []Stat
	svc := elasticache.New(session.New(&aws.Config{
		Region: aws.String(os.Getenv("REGION")),
	}))
	eparams := &elasticache.DescribeCacheClustersInput{
		CacheClusterId:    aws.String(name),
		MaxRecords:        aws.Int64(20),
		ShowCacheNodeInfo: aws.Bool(true),
	}
	resp, err := svc.DescribeCacheClusters(eparams)
	if err != nil {
		fmt.Println(err)
		return stats, err
	}
	endpointhost := *resp.CacheClusters[0].CacheNodes[0].Endpoint.Address
	endpointport := *resp.CacheClusters[0].CacheNodes[0].Endpoint.Port
	stringport := strconv.FormatInt(endpointport, 10)
	fmt.Println(endpointhost + ":" + stringport)

	tcpAddr, err := net.ResolveTCPAddr("tcp4", endpointhost+":"+stringport)
	if err != nil {
		fmt.Println(err)
		return stats, err
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Println(err)
		return stats, err
	}
	_, err = conn.Write([]byte("stats\n"))
	conn.CloseWrite()
	if err != nil {
		fmt.Println(err)
		return stats, err
	}
	var resulta []string
	result, err := ioutil.ReadAll(conn)
	if err != nil {
		fmt.Println(err)
		return stats, err
	}
	resulta = strings.Split(string(result), "\n")
	for _, element := range resulta {
		if strings.HasPrefix(element, "STAT") {
			//fmt.Println(element)
			var stat Stat
			stata := strings.Split(element, " ")
			//fmt.Println(stata[0])
			//fmt.Println(stata[1])
			//fmt.Println(stata[2])
			stat.Key = stata[1]
			t := strings.TrimSpace(stata[2])

			stat.Value = t
			//fmt.Println(stat)
			stats = append(stats, stat)
		}
	}
	b, err := json.Marshal(stats)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return stats, err
	}

	fmt.Println(string(b))
	return stats, nil

}
