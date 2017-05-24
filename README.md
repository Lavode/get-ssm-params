# get-ssm-params

Get AWS SSM parameters

## Build

Clone this repository. Inside checkout:

```bash
# if using Go < 1.8
export GOPATH=/home/you/go

# fetch dependencies (aws sdk, as listed in main.go) into $GOPATH
go get -d

# build; env required if building on mac for linux
env GOOS=linux GOARCH=amd64 go build

# alternatively, if not cross compiling -- install into $GOPATH/bin using
env GIT_TERMINAL_PROMPT=1 go get -v github.com/projectThor/get-ssm-params
```

## Usage

From -h output:

```
Usage of get-ssm-params:
  -awsregion string
      [$SSM_AWS_REGION] AWS region (default "eu-central-1")
  -env string
      [$SSM_ENV] environment name to use (PROD, STAG, ...)
  -extraparams string
      [$SSM_EXTRA_PARAMS] parameters to fetch (explicit)
  -params string
      [$SSM_PARAMS] parameters to fetch (prefixes env+service)
  -service string
      [$SSM_SERVICE] service name to use (YVES, ZED, ...)
  -version
      Print version of get-ssm-params
```

### Examples

```bash
# Fetch two params for given env/service
$ get-ssm-params -env PROD -service FOOBAR -params DB_HOST,DB_USER,DB_PASS
# this will retrieve parameters named PROD_FOOBAR_DB_HOST and PROD_FOOBAR_DB_USER,
# but output will be (env/service is stripped):
DB_HOST="example.com"
DB_USER="..."

# or, get specific/explicit params (i.e. not relying on env/service)
$ get-ssm-params -extraparams BLA_BLI,BLUB_BLUB
BLA_BLI="yoyo"
BLUB_BLUB="yumyum"

# alternatively, let get-ssm-params put parameters into env and exec a 'final' entrypoint
$ export SSM_ENV=FOO
$ export SSM_SERVICE=BAR
$ export SSM_PARAMS=BLURP,BLIP
# this will exec 'npm run', with desired/retrieved params in environment:
get-ssm-params npm run
```

If one of the parameters cannot be retrieved, `get-ssm-params` will `exit(1)`.
By default, `get-ssm-params` uses AWS region `eu-central-1`. To override,
use `-awsregion` command line option or define `SSM_AWS_REGION` environment variable.

Example usage in Dockerfile:

```
## usage WITHOUT get-ssm-params

ENTRYPOINT ["/tini", "--"]
CMD ["/usr/local/bin/uwsgi", "--ini", "/config/app.ini"]

## usage WITH get-ssm-params

# copy get-ssm-params binary to root directory of image
ADD get-ssm-params .
# keep entrypoint as-is
ENTRYPOINT ["/tini", "--"]
# adjust command to let get-ssm-params exec() original command
CMD ["/get-ssm-params", "/usr/local/bin/uwsgi", "--ini", "/config/app.ini"]
```

The above example with get-ssm-params used would expect `SSM_ENV`, `SSM_SERVICE` and `SSM_PARAMS` to
be passed via regular container environment variables. Corresponding example CloudFormation stack:

```yaml
  TaskDefinition:
    Type: AWS::ECS::TaskDefinition
    ...
          Environment:
            -
              Name: SSM_ENV
              Value: dev
            -
              Name: SSM_SERVICE
              Value: myApp
            -
              Name: SSM_PARAMS
              Value: LDAPBINDPW,ACCESS_KEY
```

Corresponding parameters must be put into parameter store before container launch, i.e.

```bash
aws ssm put-parameter --name dev_myApp_LDAPBINDPW --value "APassword123" --type SecureString \
                      --key-id "...use key id..." --region eu-central-1
```

Ensure to only grant access to desired containers by following the
[AWS IAM Roles for Tasks guide](https://aws.amazon.com/blogs/compute/managing-secrets-for-amazon-ecs-applications-using-parameter-store-and-iam-roles-for-tasks/)

## Example error messages

```
NoCredentialProviders: no valid providers in chain. Deprecated.
  For verbose messaging see aws.Config.CredentialsChainVerboseErrors
```

The above message indicates that the host has no policy attached.

```
AccessDeniedException: User: arn:aws:sts::123412341234:assumed-role/myec2Role/i-c7722c4d is not authorized to perform: ssm:GetParameters on resource: arn:aws:ssm:eu-west-1:123412341234:parameter/testval
  status code: 400, request id: df465e18-109d-11e7-bfc5-4f01fca110c2
```

The above message indicates that the given ec2 host has a policy attached,
but it lacks permission on requested parameters.

## Links

 - https://aws.amazon.com/blogs/compute/managing-secrets-for-amazon-ecs-applications-using-parameter-store-and-iam-roles-for-tasks/
 - http://docs.aws.amazon.com/systems-manager/latest/userguide/sysman-paramstore-walk.html
 - https://godoc.org/github.com/aws/aws-sdk-go/service/ssm#GetParametersInput
 - https://godoc.org/github.com/aws/aws-sdk-go/service/ssm#example-SSM-GetParameters
 - http://docs.aws.amazon.com/cli/latest/reference/ssm/get-parameters.html
 - https://github.com/aws/amazon-ssm-agent/blob/master/agent/parameters/parameters.go
 - https://github.com/aws/amazon-ssm-agent/blob/master/agent/parameterstore/parameterstore.go
