# Sealr

Sealr is the reusable core library behind the gitvault CLI. It keeps the
hexagonal architecture intact and exposes the same services with pluggable
adapters.

## Package layout

- `sealr/domain`: value types and parsing helpers
- `sealr/ports`: interfaces for side effects
- `sealr/services`: application services
- `sealr/infra`: default OS adapters (SOPS, git, filesystem, clock)

## Default system

Use the default system when you want the same behavior as the CLI:

```go
package main

import (
	"context"
	"fmt"

	"github.com/aatuh/sealr"
	"github.com/aatuh/sealr/services"
)

func main() {
	ctx := context.Background()
	system := sealr.NewDefaultSystem()

	err := system.InitService.Init(ctx, services.InitOptions{
		Root:       "./vault",
		Name:       "my-vault",
		Recipients: []string{"age1example..."},
		InitGit:    true,
	})
	if err != nil {
		panic(err)
	}

	if err := system.SecretService.Set(ctx, "./vault", "myapp", "dev", "API_KEY", "abc123"); err != nil {
		panic(err)
	}

	data, err := system.SecretService.ExportEnv(ctx, "./vault", "myapp", "dev")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
}
```

## Custom dependencies

For tests or alternate backends, wire your own adapters with `NewSystem`:

```go
system, err := sealr.NewSystem(sealr.Dependencies{
	FS:        myFileSystem,
	Encrypter: myEncrypter,
	Git:       myGit,
	Clock:     myClock,
})
if err != nil {
	panic(err)
}
```
