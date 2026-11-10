// Package fgatest runs an in-memory OpenFGA for integration tests.
package fgatest

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openfga/go-sdk/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Image is the OpenFGA image the harness runs.
const Image = "openfga/openfga:v1.22.1"

var (
	apiURL    string
	storeSeqs atomic.Int64
)

// Main starts the shared container around a package's tests.
func Main(m *testing.M) {
	ctx := context.Background()
	container, err := testcontainers.Run(ctx, Image,
		testcontainers.WithCmd("run"),
		testcontainers.WithExposedPorts("8080/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/healthz").WithPort("8080/tcp").WithStartupTimeout(time.Minute)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start openfga: %v\n", err)
		os.Exit(1)
	}
	endpoint, err := container.PortEndpoint(ctx, "8080/tcp", "http")
	if err != nil {
		fmt.Fprintf(os.Stderr, "openfga endpoint: %v\n", err)
		_ = testcontainers.TerminateContainer(container)
		os.Exit(1)
	}
	apiURL = endpoint
	code := m.Run()
	if err := testcontainers.TerminateContainer(container); err != nil {
		fmt.Fprintf(os.Stderr, "terminate openfga: %v\n", err)
	}
	os.Exit(code)
}

// APIURL is the running container's HTTP endpoint.
func APIURL() string { return apiURL }

// StoreName is unique per call.
func StoreName(t *testing.T) string {
	return t.Name() + "-" + strconv.FormatInt(storeSeqs.Add(1), 10)
}

// Client is unbound to any store.
func Client(t *testing.T) *client.OpenFgaClient {
	t.Helper()
	fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: apiURL})
	if err != nil {
		t.Fatal(err)
	}
	return fga
}

// NewStore creates an empty store and returns an unpinned client bound to it.
func NewStore(ctx context.Context, t *testing.T) (fga *client.OpenFgaClient, storeID string) {
	t.Helper()
	fga = Client(t)
	res, err := fga.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: StoreName(t)}).Execute()
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := fga.SetStoreId(res.Id); err != nil {
		t.Fatal(err)
	}
	return fga, res.Id
}
