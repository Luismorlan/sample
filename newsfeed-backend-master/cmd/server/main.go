package main

import (
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rnr-capital/newsfeed-backend/server"
	"github.com/rnr-capital/newsfeed-backend/server/middlewares"
	"github.com/rnr-capital/newsfeed-backend/utils/dotenv"
	. "github.com/rnr-capital/newsfeed-backend/utils/flag"
	. "github.com/rnr-capital/newsfeed-backend/utils/log"
	gintrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gin-gonic/gin"
)

func init() {
	// Middlewares
	middlewares.Setup()

	LogV2.Info("api server initialized")
}

func cleanup() {
	LogV2.Info("api server shutdown")
}

func main() {
	ParseFlags()

	defer cleanup()

	if err := dotenv.LoadDotEnvs(); err != nil {
		panic(err)
	}

	// Default With the Logger and Recovery middleware already attached
	router := gin.Default()

	router.Use(cors.Default())
	router.Use(gintrace.Middleware(*ServiceName))
	if !*ByPassAuth {
		router.Use(middlewares.JWT())
	}

	handler := server.GraphqlHandler()
	router.POST("/api/graphql", handler)
	router.GET("/api/subscription", handler)

	router.GET("/api/healthcheck", server.HealthcheckHandler())

	// Setup graphql playground for debugging
	router.GET("/playground", func(c *gin.Context) {
		playground.Handler("GraphQL", "/api/graphql").ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/playground/sub", func(c *gin.Context) {
		playground.Handler("Subscription", "/api/subscription").ServeHTTP(c.Writer, c.Request)
	})

	router.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"message": "Newsfeed server - API not found"})
	})

	LogV2.Info("api server starts up")
	router.Run(":8080")
}
