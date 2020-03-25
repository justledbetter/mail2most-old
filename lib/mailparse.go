package mail2most
//
// mail2most Mail Parser
//
// This extension to the mail2most app is intended to pull the "latest reply" off of the top of incoming
// e-mail messages, and only post that to the Mattermost channel. We do this to keep the Mattermost
// channel looking conversational. Otherwise, the user is spammed with entire copies of an e-mail
// conversation, including repetitive replies that may have already been posted to the channel (as they
// are included in the full text of the e-mail by default).
//

import (
	"os"
	"fmt"
	"bufio"
	"errors"
	"regexp"
	"crypto/sha256"

	// image extensions
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Track attachments that we have seen within an import session. Once we've posted an attachment to
// the channel once, we don't want to upload it again. This is to clean up channel text where users
// are in the habit of including the same attachments every time they hit Reply All in their mail
// client.
//
var seenAttachments map[[32]byte]string

// parseHtml attempts to strip everything out of the message body except for the latest reply. This is
// not perfect, but it's better than nothing. Different mail clients encode their message and replies
// in their own unique ways, and it's impossible to account for all of the potential variations.
//
// Returns the stripped message body, or null and an error.
//
func (m Mail2Most) parseHtml( b []byte ) ([]byte, error) {

	sum := sha256.Sum256(b)
	var f *os.File
	var e error
	if _, err := os.Stat(fmt.Sprintf("outputs/%x.in",sum)); os.IsNotExist(err) {
		if f, e = os.Create(fmt.Sprintf("outputs/%x.in",sum)); e == nil {
			writer := bufio.NewWriter(f)
			_, e = writer.Write(b)
			if e != nil {
				fmt.Println( " >>> debug thing went boom ", e);
			}
			writer.Flush()
			f.Close()
		} else {
			fmt.Println( " >>> debug thing went boom ", e);
		}
	}

	// Is this an error message?  Nuke it.
	NI := regexp.MustCompile(`An error occurred while trying to deliver the mail to the following recipients:`)
	if NI.Match(b) {
		return []byte{}, errors.New("Ignoring postal service error")
	}

	// Kill the beginning because we don't need it.
	hb := regexp.MustCompile(`(?i)<html.*/head>`)
	b = hb.ReplaceAll(b,[]byte(""))

	// I hate Microsoft.
	MS := regexp.MustCompile(`<div style="border-top:solid[^>]*?><p[^>]*?><strong><span[^>]*>[A-Za-z]+:.*`)
	b = MS.ReplaceAll(b,[]byte(""))

	xs := regexp.MustCompile(` ?xmlns:?[a-z]+?="[^"]*?"`)
	b = xs.ReplaceAll(b,[]byte(""))

	// Remove ALL newlines
	nl := regexp.MustCompile(`[\r\n]+`)
	b = nl.ReplaceAll(b,[]byte(""))

	// In the off chance their e-mail client does the "On X Y Z, <user@email> said:", get rid of that junk first.
	ow := regexp.MustCompile(`On .*? wrote:.*`)
	b = ow.ReplaceAll(b,[]byte(""))

	// Remove all <!--[MSO COMMENTS]--> and their contents
	MC := regexp.MustCompile(`<!--\[if.*?endif]-->`)
	b = MC.ReplaceAll(b,[]byte(""))

	// Remove all <!-- comments --> and their contents
	co := regexp.MustCompile(`<!--.*?-->`)
	b = co.ReplaceAll(b,[]byte(""))

	// If someone forwards a message into the group, we'd like to hide that fact.
	fw := regexp.MustCompile(`Begin forwarded message:`)
	b = fw.ReplaceAll(b,[]byte(""))

	// Remove all &nbsp;s
	nb := regexp.MustCompile(`&nbsp;?`)
	b = nb.ReplaceAll(b,[]byte(""))

	// Remove all <style> tags and their contents
	st := regexp.MustCompile(`(?i)<style.*?>.*?</style>`)
	b = st.ReplaceAll(b,[]byte(""))

	// Remove all <meta> tags and their contents
	me := regexp.MustCompile(`(?i)<meta.*?>.*?</meta>`)
	b = me.ReplaceAll(b,[]byte(""))
	m2 := regexp.MustCompile(`(?i)<meta.*?/>`)
	b = m2.ReplaceAll(b,[]byte(""))

	// Kill <div> tags and end tags (but not the contents)
	d1 := regexp.MustCompile(`(?i)<div[^>]*?>`)
	b = d1.ReplaceAll(b,[]byte(""))
	d2 := regexp.MustCompile(`(?i)</div>`)
	b = d2.ReplaceAll(b,[]byte(""))

	// Remove all <o:p> tags and their contents
	op := regexp.MustCompile(`(?i)<o:p[^>]*>[^>]*</o:p>`)
	b = op.ReplaceAll(b,[]byte(""))

	// Remove all style attributes from every tag
	sa := regexp.MustCompile(`(?i) ?style="[^"]*"`)
	b = sa.ReplaceAll(b,[]byte(""))

	// Remove all style attributes from every tag
	cl := regexp.MustCompile(`(?i) ?class="[^"]*"`)
	b = cl.ReplaceAll(b,[]byte(""))

	// Remove annoying headers
	sr := regexp.MustCompile(`(?i)(<blockquote[^>]*>)?<(strong|b)>[^:]*: ?</(strong|b)> ?[^<]+<br/?>`)
	b = sr.ReplaceAll(b,[]byte(""))

	// We don't care about nowrap
	nw := regexp.MustCompile(` ?nowrap="[^"]*"`)
	b = nw.ReplaceAll(b,[]byte(""))

	// Remove all <span> tags and leave their contents
	sp := regexp.MustCompile(`<span>(.*?)</span>`)
	b = sp.ReplaceAll(b,[]byte("$1"))

	// Remove all <img> tags that don't point to websites.
	im := regexp.MustCompile(`<img.+src="[^h][^t][^>]*?>`)
	b = im.ReplaceAll(b,[]byte(""))

	// Remove excessive <br>s
	br := regexp.MustCompile(`(<br[^>]*?>){2,}`)
	b = br.ReplaceAll(b,[]byte("<br>"))

	// Kill tables for now.
//	tb := regexp.MustCompile(`<table(.*)?/table>`)
//	b = tb.ReplaceAll(b,[]byte("<p><i>(Table removed to improve readability. Please use attachments.)</i></p>"))

//	// Simplify <td> elements
	tp := regexp.MustCompile(`<td([^>]*?)><p[^>]*>(.*?)</p></td>`)
	b = tp.ReplaceAll(b,[]byte("<td$1>$2</td>"))

	// If we have 4 or more empty <p>s in a row, this is a good place to stop the message.
	MK := regexp.MustCompile(`(?i)<p></p>{4,}.*`)
	b = MK.ReplaceAll(b,[]byte(""))

	// Remove empty <p>s
	pp := regexp.MustCompile(`(?i)<p></p>`)
	b = pp.ReplaceAll(b,[]byte(""))

	// Straggling <blockquotes>
	sb := regexp.MustCompile(`(?i)<blockquote[^>]*>$`)
	b = sb.ReplaceAll(b,[]byte(""))

	// Finally, if we're lucky enough to have a "Sent from" footer to the reply, kill everything else.
	// (If not, Heaven help the poor user.)
	sf := regexp.MustCompile(`(Sent [Ff]rom|Sent via).*`)
	b = sf.ReplaceAll(b,[]byte(""))

	if _, err := os.Stat(fmt.Sprintf("outputs/%x.out",sum)); os.IsNotExist(err) {
		if f, e = os.Create(fmt.Sprintf("outputs/%x.out",sum)); e == nil {
			writer := bufio.NewWriter(f)
			_, e = writer.Write(b)
			if e != nil {
				fmt.Println( " >>> debug thing went boom ", e);
			}
			writer.Flush()
			f.Close()
		} else {
			fmt.Println( " >>> debug thing went boom ", e);
		}
	}

	return b, nil
}

// parseText attempts to strip everything out of the text/plain message body except for the latest reply.
//
func (m Mail2Most) parseText( b []byte ) ([]byte, error) {

	on := regexp.MustCompile(`(?s)On .*? wrote:.*$`)
	b = on.ReplaceAll(b,[]byte(""))

	fw := regexp.MustCompile(`(?i)Begin forwarded message:`)
	b = fw.ReplaceAll(b,[]byte(""))

	ws := regexp.MustCompile(`(?s)\s{2+}`)
	b = ws.ReplaceAll(b,[]byte(" "))

	re := regexp.MustCompile(`(.+): ((.|\r\n\s)+)\r\n`)
	b = re.ReplaceAll(b,[]byte(""))

	return b, nil
}

// parseAttachment packages an attachment in an Attachment{} type object for consumption elsewhere.
//
func (m Mail2Most) parseAttachment( body []byte, header string ) (Attachment, error) {

	filename := "image"

	fn := regexp.MustCompile(`name="([^"]+)"`)
	f := fn.FindStringSubmatch(header)

	im := regexp.MustCompile(`image/([a-z]*)`)
	cn := im.FindStringSubmatch(header)
	if len(f) > 0 {
		filename = f[1]
	} else if len(cn) > 0 {
		filename = "image." + cn[1]
	}

	sum := sha256.Sum256(body)
	if _, ok := seenAttachments[sum]; ok {
		return Attachment{}, errors.New("seenseenseen")
	}

	seenAttachments[sum] = filename
	return Attachment{Filename: filename, Content: body}, nil
}
