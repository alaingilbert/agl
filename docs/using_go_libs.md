# Guide on using Go libraries

We assume that `agl` tool is available in your `$PATH`  

Otherwise, build and install it  
```sh
cd /path/to/agl
go build
mv agl /usr/local/bin
```

- Make a new project directory "myProject"
- Initialize a module named "myProject" `agl mod init myProject`
- Get an external dependency that we want to use the project `go get ...`
- Vendor our external dependencies `agl mod vendor`
- Run the script `agl run main.agl`

```sh
mkdir myProject && cd myProject
agl mod init myProject
go get github.com/google/uuid
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