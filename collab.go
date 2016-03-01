package test161

var collabMsgs = map[string]string{
	"asst1": CollabMsgAsst1,
	"asst2": CollabMsgAsst2,
}

const CollabMsgAsst1 = `
 - Pair programming to complete the ASST1 implementation tasks is strongly encouraged.

 - Answering the ASST1 code reading questions side-by-side with your partner is 
   strongly encouraged.
 
 - Helping other students with Git, GDB, editing, and with other parts of the OS/161
   toolchain is encouraged.

 - Discussing the code reading questions and browsing the source tree with other students 
   is encouraged.

 - Dividing the code reading questions and development tasks between partners is discouraged.

 - Copying any answers from anyone who is not your partner or anywhere else and submitting
   them as your own is cheating.

 - You may not refer to or incorporate any external sources without explicit permission.

`

const CollabMsgAsst2 = `
 - Pair programming to complete the implementation tasks is strongly encouraged.

 - Writing a design document with your partner is strongly encouraged.

 - Having one partner work on the file system system calls while the other
   partner works on process support is a good division of labor. The partner
   working on the file system system calls may finish first, at which point they
   can help and continue testing.

 - Answering the code reading questions side-by-side with your partner is  strongly
   encouraged.

 - Discussing the code reading questions and browsing the source tree with other
   students is encouraged.

 - Dividing the code reading questions and development tasks between partners is
   discouraged.

 - Any arrangement that results in one partner writing the entire design document is
   cheating.

 - Copying any answers from anyone who is not your partner or anywhere else and submitting
   them as your own is cheating.

 - You may not refer to or incorporate any external sources without explicit permission.

`
