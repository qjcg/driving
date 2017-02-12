Generate reports from RedHat Training survey files.


# Features

- generate reports in several formats
    - text
    - HTML
    - PNG
- save reports and view history


# Usage

## Text report

```
$ driving < survey-20160915.txt
Responses     5
Curriculum    3.90
Instructor    4.30
Environment   0.00
Overall       3.80
NPS           0.00
```

## HTML report

```
$ driving -f html < survey-20160915.txt
```

## Debug mode

```
$ driving -d < survey-20160915.txt
2017-02-11 9:30pm [DEBUG] Survey 1: Curriculum 3.90 Instructor 4.30 Environment 0.00 Overall 3.80
2017-02-11 9:31pm [DEBUG] Survey 2: Curriculum 3.90 Instructor 4.30 Environment 0.00 Overall 3.80
2017-02-11 9:32pm [DEBUG] Survey 3: Curriculum 3.90 Instructor 4.30 Environment 0.00 Overall 3.80
2017-02-11 9:33pm [DEBUG] Survey 4: Curriculum 3.90 Instructor 4.30 Environment 0.00 Overall 3.80
2017-02-11 9:34pm [DEBUG] Survey 5: Curriculum 3.90 Instructor 4.30 Environment 0.00 Overall 3.80
Responses     5
Curriculum    3.90
Instructor    4.30
Environment   0.00
Overall       3.80
NPS           0.00
```


# License

MIT.
