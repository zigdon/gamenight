import os
import sys
sys.path.append(os.path.join(os.path.dirname(__file__), 'lib'))

from datetime import datetime
from dateutil import parser
import jinja2
import logging
import os
import urllib
import webapp2

from google.appengine.api import users

from schema import Gamenight, Invitation, User
from utils import Utils


JINJA_ENVIRONMNT = jinja2.Environment(
  loader=jinja2.FileSystemLoader(os.path.dirname(__file__)),
  extensions=['jinja2.ext.autoescape'])

def logged_in(func):
    def dec(self, **kwargs):
        sys_user = users.get_current_user()
        if not sys_user:
            self.redirect(users.create_login_url(self.request.uri))
            return

        return func(self, **kwargs)
    return dec


class MainPage(webapp2.RequestHandler):
    def get(self):
        futurenights = Gamenight.future(10)

        if futurenights and futurenights[0].this_week():
            # we have a gamenight scheduled for this week
            gamenight = futurenights[0]
            if len(futurenights) > 1:
                futurenights = futurenights[1:]
            else:
                futurenights = None
        else:
            gamenight = Gamenight.schedule()

        updated = gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p')
        template_values = {
          'future': futurenights,
          'status': gamenight.status,
          'updated': updated,
        }

        if gamenight.date:
            template_values['when'] = gamenight.date.strftime('%I:%M %p')

        if gamenight.location:
            template_values['where'] =  gamenight.location

        template = JINJA_ENVIRONMNT.get_template('index.html')
        # Write the submission form and the footer of the page
        self.response.write(template.render(template_values))


class EditPage(webapp2.RequestHandler):
    @logged_in
    def get(self, template_values={}):
        user = User.get_or_insert(users.get_current_user().email())

        futurenights = Gamenight.future(100)
        if user.superuser:
            invitations = Invitation.query()
        else:
            invitations = Invitation.query(Invitation.owner==user.key)
        invitations = invitations.order(Invitation.date).iter()

        summary = Invitation.summary()

        template_values.update({
            'scheduled': futurenights,
            'invitations': invitations,
            'summary': summary,
            'logout': users.create_logout_url('/'),
            'user': user.key.id(),
            'admin': user.superuser,
            'users': User.query().iter(),
        })

        template = JINJA_ENVIRONMNT.get_template('edit.html')
        self.response.write(template.render(template_values))


class InvitePage(webapp2.RequestHandler):
    @logged_in
    def get(self, template_values={}, msg=None, error=None):
        user = User.get_or_insert(users.get_current_user().email())

        if user.superuser:
            invitations = Invitation.query()
        else:
            invitations = Invitation.query(Invitation.owner==user.key)
        invitations = invitations\
                      .filter(Invitation.date >= Utils.Now())\
                      .order(Invitation.date).iter()

        template_values.update({
            'user': user,
            'msg': msg,
            'error': error,
            'invitations': invitations,
            'logout': users.create_logout_url('/'),
        })

        template = JINJA_ENVIRONMNT.get_template('invite.html')
        self.response.write(template.render(template_values))

    @logged_in
    def post(self, template_values={}):
        user = User.get_or_insert(users.get_current_user().email())

        if self.request.get('withdraw'):
            invite = Invitation.get(self.request.get('withdraw'))
            if not invite:
                self.get(error="Can't find this invitation.")
                return

            if invite.owner != user.key and not user.superuser:
                self.get(error="Not your invitation.")
                return

            msg = ''
            gn = Gamenight.query(Gamenight.invitation==invite.key).get()
            if gn:
                gn.key.delete()
                msg = 'Rescheduling gamenight. '
                # TODO: actually reschedule the gn

            invite.key.delete()
            msg += 'Invitation withdrawn.'

            self.get(msg=msg)
            return


        args = {}
        for k in ['when', 'where', 'priority', 'notes']:
            args[k] = self.request.get(k)

        error = None
        if args['when']:
            try:
                args['when'] = parser.parse(args['when'])
            except ValueError:
                error = 'Not sure what you mean by "%s"' % args['when']
        else:
            error = 'When do you want to host?'

        if error:
            self.get(template_values=args, error=error)
            return

        if not args['where']:
            error = 'Where do you want to host?'
            self.get(template_values=args, error=error)
            return

        if not args['priority']:
            error = '''Gotta have a priority. Also, don't mess with me.'''
            self.get(template_values=args, error=error)
            return

        args['owner'] = user.key

        Invitation.create(args)

        self.get(msg="Invitation created!")


application = webapp2.WSGIApplication([
    ('/', MainPage),
    ('/edit', EditPage),
    ('/invite', InvitePage),
], debug=True)

# vim: set ts=4 sts=4 sw=4 et:
