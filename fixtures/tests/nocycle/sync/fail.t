---
name: Test a failure
tags:
 - sync
depends:
 - threads
stat:
    resolution: 0.01
    window: 100
misc:
    prompttimeout: 30.0
---
sem1
panic
lt1
cvt1
