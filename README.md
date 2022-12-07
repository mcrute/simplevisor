# Yet Another Process Supervisor

Simplevisor is yet another process supervisor but with a very narrow
scope and a specific set of requirements that caused its creation. If
you want a full-featured process supervisor you probably want something
other than this, but if you share these requirements then you may have
found what you need.

The requirements for Simplevisor are:

* Be a suitable PID 1 for containers (handle signals and shutdown)
* Allow running processes as non-root users
* Allow running jobs that initialize the environment
* Keep jobs alive
* Support consolidated JSON logging for all sub-processes
* Allow re-mapping termination signals
* Support integration with [Vault](https://www.vaultproject.io/) for
  Vault-unaware applications (bonus points for 12-Factor patterns)

And this is what Simplevisor does. Nothing more, nothing less.

## Still To Do
* Process restarts with backoff do not work
* Signals need better testing
* Vault token passing does not work
* Support processes that already log in JSON format

## Usage
Simplevisor tries to be very simple to use. If the config file
``simplevisor.json`` is in the same directory as the binary then running
``./simplevisor`` as root with some Vault environment variables is
sufficient to start all of the processes and keep them running.

Vault integration is enabled by default, but can be disabled by
passing ``--no-vault``. If Vault integration is enabled then the
following variables must be present in the process environment.
``VAULT_ADDR``, which must contain a URL pointing to Vault. One of
either ``VAULT_TOKEN``, containing a token to use when authenticating
Vault calls, or both of ``VAULT_ROLE_ID`` and ``VAULT_SECRET_ID`` when
using [AppRole](https://developer.hashicorp.com/vault/docs/auth/approle)
authentication.

In addition to disabling Vault integration ``--config`` can be passed to 
provide a non-standard location for the config file.

## Configuration
The configuration file format is JSON and is well documented (for full
details see: supervisor/model.go). Here is a worked example.

### Environment
By default environment variables are only passed through to subprocesses
if they are on the ``pass`` list. This prevents leaking secret
environment variables meant for Simplevisor itself through to the
managed processes. This can be overridden by setting ``pass-all`` to
``true``. All variables to be passed through to the managed process must
be in ``pass``, variables in the Vault specific lists do not imply their
existence in ``pass``.

### Vault
***IMPORTANT:*** The Vault integration requires periodic login tokens.
Not using periodic tokens will cause Simplevisor to eventually fail to
renew the credential lease and terminate all managed processes.

Vault integration works by looking for variables in the Simplevisor
environment named in the ``vault-replace`` and ``vault-template`` keys.
If keys are found that match ``vault-replace`` they will be parsed,
looked up in Vault, and the returned value will be injected into the
environment of managed processes. This will happen once at Simplevisor
startup time and each managed process will see the same secrets.

Replacement variables are colon (``:``) separated lists of three
arguments: type, path, and field. Type is one of ``db`` or ``secret``
where ``db`` refers to database credentials (mounted at ``database/`` in
Vault) and ``secret`` refers to JSON formatted KV secrets (mounted at
``kv/`` in Vault). The path is the path within the mount-point to the
secret. For database credentials the field can be one of ``Username``
or ``Password`` and nothing else. For secret type credential the field
refers to the key in the returned JSON document, only single-level JSON
documents containing string keys and values are supported.

Credentials are cached upon first fetch and subsequent references to
them will used the cached value. This presents a consistent view of
the secrets to the process regardless of how many times the secret is
referred to in the environment.

Simplevisor will manage renewing the login token and any credential
leases that are acquired as part of the environment expansion described
here.

For example, given the configuration below and the following environment
state when Simplevisor is launched:

```sh
MONGO_USER="db:prod-db:Username"
MONGO_PASSWORD="db:prod-db:Password"
DJANGO_SECRET="secret:my/app/django-secret:Secret"
```

Assuming that ``kv/v1/my/app/django-secret`` contains:

```json
{ "Secret": "some-secret" }
```

Managed processes would observe the following in their environment:

```sh
MONGO_USER="some-username-for-prod-db"
MONGO_PASSWORD="password-for-the-above-username"
DJANGO_SECRET="some-secret"
```

### Vault Templates
Templates are designed to allow creation of more complex
secrets based on other secrets, such as JDBC connection
strings. The ``vault-template`` list contains variables that,
if found in the Simplevisor environment, will be parsed as Go
[text/template](https://pkg.go.dev/text/template) templates and rendered
with a context of the resolved variables in ``vault-replace``. Note that
the templates will not have access to the full environment.

For example the following template, evaluated with the variables from
the prior section:

```sh
MONGO_URL="mongodb://{{ .MONGO_USER }}:{{ .MONGO_PASSWORD }}@my-mongo-host.prod:27017/prod-database?authSource=admin"
```

This would result in managed processes observing:

```sh
MONGO_URL="mongodb://some-username-for-prod-db:password-for-the-above-username@my-mongo-host.prod:27017/prod-database?authSource=admin"
```

### Vault Token
The supervisor Vault token can be injected to managed processes as
``VAULT_TOKEN``. This implies that ``VAULT_ADDR`` will also be injected.

### Job Configuration
There are two types of jobs ``init`` jobs and ``main`` jobs. They differ
only in when and how they are run and if they are restarted on failure.
All jobs receive an identical environment, per the preparation noted
above.

``init`` jobs are run serially before the ``main`` jobs are started.
They are expected to exit with a zero status code. The failure of
an ``init`` job will result in the termination of all jobs and the
supervisor exiting with error.

``main`` jobs are configured identically to ``init`` jobs but are
started in parallel after the ``init`` jobs complete and are restarted
if they fail. Failure of a ``main`` job does not terminate the
supervisor. Failing ``main`` jobs are restarted with exponential
backoff.

Each job will be spawned as the leader of its own session and will
run as the configured user and group. If user and group are not
specified then ``root:root`` is assumed. If user is specified but group
is not then ``root`` is assumed. The ``run-as`` key takes the form
``user:group``.

When the process supervisor is shutting down it will send a
``TERM`` signal to all managed processes. This can be configured
with the ``kill-signal`` flag which should be the 
[signal name](https://www.man7.org/linux/man-pages/man7/signal.7.html)
 without the ``SIG`` prefix.

### Full Config Example
```json
{
    "env": {
        "pass": [
            "PATH",
            "HOME",
            "PWD",
            "MONGO_URL"
        ],
        "vault-token": false,
        "pass-all": false,
        "vault-replace": [
            "DB_USERNAME",
            "DB_PASSWORD",
            "DJANGO_SECRET_KEY"
        ]
        "vault-template": [
            "MONGO_URL"
        ]
    },
    "jobs": {
        "init": [
            {
                "cmd": ["/setup-env.sh"],
                "run-as": "netbox"
            }
        ],
        "main": [
            {
                "name": "queue-worker",
                "cmd": ["/usr/bin/python3", "/opt/netbox/netbox/manage.py", "rqworker"],
                "run-as": "netbox"
            },
            {
                "cmd": ["/usr/sbin/uwsgi", "--ini", "/etc/uwsgi/netbox.ini"],
                "kill-signal": "INT",
                "run-as": "root"
            }
        ]
    }
}
```

## Logging
When a managed process writes to either stdout or stderr those log
messages will be captured, attributed to the process, and logged to the
stdout of Simplevisor as JSON. The JSON format contains the fields:

* ``process``: the name of the process, either as configured in the
  config file or the basename of the first argument in the command. If
  the command name is not descriptive or ambiguous, set the ``name`` in
  the job configuration. This is not mandatory but will make reading logs
  easier.
* ``time``: the Unix timestamp of the log entry in integral format
* ``stream``: an integer indicating the stream the process wrote to 0
  for stdout, 1 for stderr.
* ``message``: one line of the message written by the process

Writing multiple lines will result in multiple log messages (as can be
the case for stack traces). Lines are always newline terminated.

Example:
```json
{"process":"bash","time":1670346907,"stream":0,"message":"..."}
{"process":"internal","time":1670349781,"stream":0,"message":"..."}
```

## But Why?
This all seems pretty complex and a lot of moving pieces,
and in a sense it is. This is also a major simplification
of what existed before. Prior to Simplevisor we used an
amalgamation of [dumb-init](https://github.com/Yelp/dumb-init),
[runit](http://smarden.org/runit/index.html), and
[su-exec](https://github.com/ncopa/su-exec) with a liberal amount of
shell scripting and, in more complex cases, custom process wrappers
to properly integrate processes with our infrastructure systems. In a
few cases third-party applications required us to carry local patches
anywhere from 50-1000 lines to support this which added a lot of burden
in upgrading these applications and in some cases were very fragile.

Simplevisor has fixed all of this. It's eliminated all first-party
patches, most of the shell scripts, and all of those extra tools in
favor of one small binary (~15Mb on disk and ~13Mb RAM at steady-state)
and a config file added to our application distributions.

Can something else do this? Probably. There are a lot of process
supervisors out there and this one certainly isn't the best but we think
the consolidated logging as structured JSON and Vault integration really
set this one apart.

## Contributing

We would very much appreciate bug fixes and other contributions provided
they fit of the stated goals of the project.

If you find anything here useful and would like to submit patches please email
the patch (in git format-patch format) or a repository location and branch name
that the maintainers can pull and merge. We reserve the right to request
changes to patches or reject them outright but are most likely to willing and
thankfully merge them if they fit into the general theme here.

## Contributors

* [Mike Crute](https://mike.crute.us) email: mike-at-crute-dot-us
