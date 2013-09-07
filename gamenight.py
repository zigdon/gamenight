import os
import sys
sys.path.append(os.path.join(os.path.dirname(__file__), 'lib'))

from collections import defaultdict
from datetime import datetime, timedelta
from dateutil import parser
import jinja2
import logging
import os
import urllib
import webapp2

from google.appengine.api import mail, users

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
                futurenights = []
        else:
            gamenight = Gamenight(status='Probably',
                             date=Utils.Saturday(),
                             lastupdate=Utils.Now())
            futurenights = []

        updated = gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p')

        invites = Invitation.summary()

        upcoming = {g.date: { 'type': 'gamenight',
                              'date': g.date,
                              'location': g.location }
                            for g in futurenights}
        for week in range(1,5):
            day = (Utils.Saturday() + timedelta(week*7)).date()

            # skip days we already have a confirmed GN
            if upcoming.get(day, False):
                continue

            summary = invites.get(day, False)
            if summary:
                upcoming[day] = { 'type': 'invites',
                                  'date': day,
                                  'invitations': summary }
                continue

            upcoming[day] = { 'date': day }

        template_values = {
          'future': sorted(upcoming.values(), key=lambda x: x['date']),
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


class SchedulePage(webapp2.RequestHandler):
    def get(self):
        days = defaultdict(dict)
        for gn in Gamenight.future(10):
            days[gn.date]['date'] = gn.date
            days[gn.date]['scheduled'] = True
            days[gn.date]['owner'] = gn.owner
            days[gn.date]['time'] = gn.time
            days[gn.date]['where'] = gn.location

        invitations = Invitation.query(Invitation.date >= Utils.Now()).\
                          order(Invitation.date).iter()
        for invite in invitations:
            if not days[invite.date].get('invitations'):
                days[invite.date]['date'] = invite.date
                days[invite.date]['invitations'] = []

            days[invite.date]['invitations'].append(invite)

        day_sorter = lambda x: x.get('date')
        template_values = { 'days': sorted(days.values(), key=day_sorter) }
        current_user = users.get_current_user()
        if current_user:
            user = User.get_or_insert(users.get_current_user().email())
            template_values.update({
                'logout': users.create_logout_url('/'),
                'user': user,
            })

        template = JINJA_ENVIRONMNT.get_template('schedule.html')
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
            invitations = Invitation.query(ancestor=Invitation.dummy())
        else:
            invitations = Invitation.query(Invitation.owner==user.key,
                                           ancestor=Invitation.dummy())
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

        if not template_values.has_key('where') and user.location:
            template_values['where'] = user.location

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
                self.get(error='Not your invitation.')
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

        updated = Invitation.create(args)

        if updated:
            self.get(msg='Invitation updated!')
        else:
            self.get(msg='Invitation created!')


class ProfilePage(webapp2.RequestHandler):
    @logged_in
    def get(self, template_values={}, msg=None, error=None, profile=None):
        user = User.get_or_insert(users.get_current_user().email())

        template_values.update({
            'user': user,
            'msg': msg,
            'error': error,
            'logout': users.create_logout_url('/'),
        })

        template_values['profile'] = user

        if user.superuser:
            template_values['users'] = User.query().fetch()
            if profile:
                template_values['profile'] = User.get(profile)

                if not profile:
                    template_values['profile'] = user
                    template_values['error'] = "Couldn't find user %s" % profile


        template = JINJA_ENVIRONMNT.get_template('profile.html')
        self.response.write(template.render(template_values))

    @logged_in
    def post(self):
        user = User.get_or_insert(users.get_current_user().email())

        if user.superuser:
            edit = self.request.get('edit', False)
            if edit:
                self.get(profile=edit, msg='Editing %s' % edit)
                return
            profile = User.get(self.request.get('pid'))
            profile.superuser = self.request.get('admin')=='on'
        else:
            profile = user

        profile.location = self.request.get('location')
        profile.name = self.request.get('name')
        profile.nag = self.request.get('nag')=='on'
        profile.put()

        self.get(msg='Profile updated!', profile=profile.key.id())


# tasks
class ResetTask(webapp2.RequestHandler):
    def get(self):
        Gamenight.schedule()


class NagTask(webapp2.RequestHandler):
    def get(self):
        gn = Gamenight.schedule(fallback='Maybe')
        if gn.status != 'Yes':
            message = mail.EmailMessage()
            message.sender = 'Gamenight <dan@peeron.com>'
            message.to = message.sender
            message.subject = 'Want to host gamenight?'
            message.body = """
Seems that no one has offered to host gamenight this week. Want to host? Go to http://TBD/invite!

Thanks!

(You asked to get these emails if no one is hosting by Tuesday morning. If you want to stop getting these, go to http://TBD/profile and uncheck the 'nag emails' option.)
"""

            message.bcc = [u.key.id() for u in User.query(User.nag==True).fetch()]
            logging.info('Sending nag email to %r', message.to)
            message.send()


debug = True
application = webapp2.WSGIApplication([
    ('/', MainPage),
    ('/edit', EditPage),
    ('/invite', InvitePage),
    ('/profile', ProfilePage),
    ('/schedule', SchedulePage),
], debug=debug)

cron = webapp2.WSGIApplication([
    ('/tasks/reset', ResetTask),
    ('/tasks/nag', NagTask),
], debug=debug)

# vim: set ts=4 sts=4 sw=4 et:
