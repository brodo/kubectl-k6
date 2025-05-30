## k6k8s completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	k6k8s completion fish | source

To load completions for every new session, execute once:

	k6k8s completion fish > ~/.config/fish/completions/k6k8s.fish

You will need to start a new shell for this setup to take effect.


```
k6k8s completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --config string   k6k8s config file path (default is $CWD/.k6k8s.yml)
      --k8scfg string   k8s config file path (default is $HOME/.kube/config)
```

### SEE ALSO

* [k6k8s completion](k6k8s_completion.md)	 - Generate the autocompletion script for the specified shell

###### Auto generated by spf13/cobra on 5-Mar-2024
