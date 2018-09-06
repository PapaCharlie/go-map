# map

Are you tired of forgetting the parameters to `xargs`? How about writing a while loop, only to have
it steal your stdin and leave you stranded. Well, wish no more! The `map` utility is designed to
serve as replacement for the more complex functions of `xargs`.

# Usage

Basic usage looks like this:
```bash
find . -name build -type d | map -j10 d 'echo Deleting $d && rm -r $d'
```
The above shows an example of parallelizing certain tasks, while keeping a sane output (each worker
locks the stdout before printing)

You can also get clever with `map -e`:
```bash
map -f hosts -e 'ssh $1 df -h' | awk '{print $1, $(NF-1)}'
```
Outputs the `<host, disk usage>` pairs
