# Guide on using Go libraries

We assume that `agl` tool is available in your `$PATH`  
Otherwise, build it with `go build`, then move it in `mv agl /usr/local/bin`  

```sh
mkdir myProject && cd myProject
agl mod init myProject
cat <<EOF > main.agl
package main

import (
  "fmt"
  "github.com/google/uuid"
)

func main() {
  id := uuid.NewString()
  uuid.Validate(id)!
  fmt.Println("UUID:", id)
}
EOF
agl mod vendor
agl run main.agl
```