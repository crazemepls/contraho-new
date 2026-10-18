# Contraho
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/jesayafn/contraho?sort=semver)

CLI tool for gathering information of projects and applications within Sonarqube server.

Fork of jesayafn/contraho, with changes:
- Now able to filter the projects or apps with --bureau prefix
- Now able to send the generated csv via email (SMTP)
- Changes on csv columns: 
    - Add overall maintainability, reliability, and vulnerability ratings of the project
    - Add latest Quality Scan Status (Failed or Passed)
    - LastAnalysisDate to be from any branch. If none found then value is null
    - Remove QualityGateID

## Without compile the code

### Search all projects or applications with password

#### Project

```shell
go run . project --host="http://localhost:9000" --username="admin" --password="admin" --filename="app-list.csv"
```

#### Application

```shell
go run . application --host="http://localhost:9000" --username="admin" --password="admin" --filename="app-list.csv"
```

### Search all projects or applications with token

#### Project

```shell
go run . project --host="http://localhost:9000" --token="squ_3131sa..." --filename="project-list.csv"
```

#### Application

```shell
go run . application --host="http://localhost:9000" --token="squ_3131sa..." --filename="app-list.csv"
```

## Options

### Global options
- `--filename`, filename of the csv output file
- `--token`, token credential will be used. Should be privileged user token.
- `--host`, server url of the Sonarqube server.
- `--username`, username will be used to access Sonarqube server. **Not recommended** and should be privileged user.
- `--password`, password will be used to access Sonarqube server. **Not recommended** and should be privileged user.

### `project`,`proj`, or `p`  argument's option
- `--unlisted-on-app`, list all projects that are not listed on any application. Can not be used with `--listed-on-app` option and onand on Sonarqube Community Edition.
- `--listed-on-app`, list all projects that are listed on any application. Can not be used with `--unlisted-on-app` option and on Sonarqube Community Edition.
- `--app [app1,app2]`, list all projects that are listed on any application defined in with this option. 
- `--bureau [prefix1,prefix2]`, list only projects whose project key starts with the given prefix (case-sensitive), e.g. `--bureau="tsc:"` keeps `tsc:a` and `tsc:b`. Can be combined with `--app`, 
`--listed-on-app` and `--unlisted-on-app`.
- `--mail-to [a@x.com,b@x.com]`, email the generated CSV after the run (needs `--filename`, `--mail-from` and `--smtp-host`). Related: `--smtp-user` (password from `$SMTP_PASSWORD`), `--smtp-tls [auto|none|starttls|tls]`, `--mail-subject`.