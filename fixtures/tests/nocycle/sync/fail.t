---
name: Test a failure
tags:
 - sync
 - rwlock
depends:
 - threads
stat:
    resolution: 0.01
    window: 100
misc:
    prompttimeout: 30.0
---
sy1
sy2
panic
sy3
sy4
