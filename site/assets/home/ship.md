## Ship

Copy the build into a new release directory, then point the
`current` symlink at it.

```sh {#ship confirm="Ship release to production?"}
cp -R "$SITE" "$PROD/releases/$RELEASE"
ln -sfn "releases/$RELEASE" "$PROD/current"
echo "current -> $(readlink "$PROD/current")"
```

```output {for=ship exit=0}
current -> releases/2026-01-02.1
```
