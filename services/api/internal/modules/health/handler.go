package health

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Response describes this process only; it does not assert database readiness.
type Response struct {
	Status  string    `json:"status"`
	Service string    `json:"service"`
	Version string    `json:"version"`
	Time    time.Time `json:"time"`
}

func Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, Response{
		Status: "ok", Service: "infrapro-api", Version: "0.1.0", Time: time.Now().UTC(),
	})
}
