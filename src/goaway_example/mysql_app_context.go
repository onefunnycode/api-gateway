package goaway_example

import (
	"gateway/src/goaway/core"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"gateway/src/goaway_example/web"
	"strings"
	"strconv"
)

type mysqlAppContext struct {
	db *sql.DB
}

func NewMqlAppContext() *mysqlAppContext {
	context := mysqlAppContext{}
	context.init()
	return &context
}

const connUrl = "root:Tc123456@tcp(rm-wz9s84w75709ryaw7o.mysql.rds.aliyuncs.com:3306)/gateway"

func (a *mysqlAppContext) init() {
	// 获取数据库
	db, _ := sql.Open("mysql", connUrl)
	db.SetMaxOpenConns(100) //最大连接数
	db.SetMaxIdleConns(50)  //最大闲置数
	db.Ping()
	a.db = db
}

type uriHost struct {
	Uri  string
	Host string
}

const (
	//查询服务前缀和主机(含端口)的对应关系
	SQL1 = `
		select a.Uri, concat(b.name, ':', b.port) as host
		from api a
			left join service b on a.service_id = b.service_id
		where a.status = 1`
	//查询服务前缀和过滤器名称的对应关系
	SQL2 = `
		select a.Uri, c.name as FilterName
		from api a
		  left join service b on a.service_id = b.service_id
		  left join filter c on c.api_id = a.api_id
		where
		  a.status = 1 and c.name is not null`
)

func (a *mysqlAppContext) VisitUriHosts(ctx *core.GaContext) {
	uriHosts := a.queryUriHosts()
	if len(uriHosts) > 0 {
		for _, uh := range uriHosts {
			filter := NewForwardFilter(uh.Uri, uh.Host)
			ctx.LoadFilter(filter)
		}
	}
}

func (a *mysqlAppContext) VisitUriFilters(ctx *core.GaContext) {
	filters := a.queryUriFilters()
	if len(filters) > 0 {
		for _, uf := range filters {
			filter := NewBaseServiceFilter(uf.Uri, uf.FilterName)
			ctx.LoadFilter(filter)
		}
	}
}

func (a *mysqlAppContext) queryUriHosts() []uriHost {
	rows, _ := a.db.Query(SQL1)
	defer rows.Close()
	var uriHosts []uriHost
	for rows.Next() {
		uh := uriHost{}
		rows.Scan(&uh.Uri, &uh.Host)
		uriHosts = append(uriHosts, uh)
	}
	return uriHosts
}

type uriFilter struct {
	Uri        string
	FilterName string
}

func (a *mysqlAppContext) queryUriFilters() []uriFilter {
	rows, _ := a.db.Query(SQL2)
	defer rows.Close()
	var uriFilters []uriFilter
	for rows.Next() {
		uf := uriFilter{}
		rows.Scan(&uf.Uri, &uf.FilterName)
		uriFilters = append(uriFilters, uf)
	}
	return uriFilters
}

const PAGE_SIZE = 50

func (a *mysqlAppContext) QueryService(
	uri string,
	desc string,
	currentPage int) web.MResult {
	hasUri := len(uri) > 0
	hasDesc := len(desc) > 0
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	var sqltext string
	if !hasUri && !hasDesc {
		sqltext = "select a.api_id as apiid, a.uri, a.`desc`, a.status from api a ORDER BY a.uri"
	}
	if hasUri && hasDesc {
		sqltext = "select a.api_id as apiid, a.uri, a.`desc`, a.status from api a where a.uri like '" + uri + "%' or a.`desc` like '%" + desc + "%' ORDER BY a.uri"
	}
	if hasUri && !hasDesc {
		sqltext = "select a.api_id as apiid, a.uri, a.`desc`, a.status from api a where a.uri like '" + uri + "%' ORDER BY a.uri"
	}
	if !hasUri && hasDesc {
		sqltext = "select a.api_id as apiid, a.uri, a.`desc`, a.status from api a where a.`desc` like '%" + desc + "%' ORDER BY a.uri"
	}

	//查询总条数
	countRow, _ := a.db.Query("select count(0) from (" + sqltext + ") t")
	defer countRow.Close()
	mPage := web.MPage{}
	if countRow.Next() {
		countRow.Scan(&mPage.TotalCount)
	}

	//计算设置分页的参数
	totalPage := (mPage.TotalCount + PAGE_SIZE - 1) / PAGE_SIZE
	if totalPage < currentPage {
		currentPage = totalPage - 1
	}
	if currentPage < 0 {
		currentPage = 0
	}
	mPage.CurrentPage = currentPage + 1
	sqltext += " limit " + strconv.Itoa(currentPage * PAGE_SIZE) + ", " + strconv.Itoa(PAGE_SIZE)

	//获取服务查询结果
	rows, _ := a.db.Query(sqltext)
	defer rows.Close()
	var services []web.Mservice
	for rows.Next() {
		ms := web.Mservice{}
		rows.Scan(&ms.Apiid, &ms.Uri, &ms.Desc, &ms.Status)
		services = append(services, ms)
	}

	//关联过滤器
	for _, service := range services {
		rows, _ := a.db.Query("select a.filter_id as filterid, a.name, a.status from filter a where a.api_id = " + strconv.Itoa(service.Apiid))
		defer rows.Close()
		for rows.Next() {
			mf := web.Mfilter{}
			rows.Scan(&mf.Filterid, &mf.Name, &mf.Status)
			service.Filters = append(service.Filters, mf)
		}
	}

	result := web.MResult{}
	result.MPage = mPage
	result.Mservicelist = services

	return result
}
