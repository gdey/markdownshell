#!/usr/bin/env mdsh
---
title: T
mdsh:
  shell: /bin/bash
  on_error: stop
  env:
    share: [PATH]
---
# Title

```sh {#a}
echo a
```

```output {for=a exit=0}
a
```

````sh
echo '```'
````

<!-- mdsh:output for="block-2" -->
| x |
<!-- /mdsh:output -->

text

```output {for=a}
orphan
```
  ~~~go
  x := 1
  ~~~
