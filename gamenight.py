import os
import sys
sys.path.append(os.path.join(os.path.dirname(__file__), 'lib'))

from collections import defaultdict
from datetime import datetime, timedelta, time
from dateutil import parser
import jinja2
import logging
import os
import urllib
import webapp2

from google.appengine.api import mail, users
from oauth2client.appengine import OAuth2Decorator

from pprint import pformat
from schema import Gamenight, Invitation, User, Config, Auth
from utils import Utils

JINJA_ENVIRONMNT = jinja2.Environment(
  loader=jinja2.FileSystemLoader(os.path.dirname(__file__)),
  extensions=['jinja2.ext.autoescape'])

config = {x.name: x.value for x in Config.query().fetch()}

decorator = OAuth2Decorator(
                client_id=config.get('client_id'),
                client_secret=config.get('client_secret'),
                scope='https://www.googleapis.com/auth/calendar')


def admin_only(func):
    def dec(self, **kwargs):
        sys_user = users.get_current_user()
        if not sys_user:
            self.redirect(users.create_login_url(self.request.uri))
            return

        user = User.lookup(sys_user.email())
        if not user or not user.superuser:
            self.redirect("/invite")
            return

        return func(self, **kwargs)
    return dec


def logged_in(func):
    def dec(self, **kwargs):
        sys_user = users.get_current_user()
        if not sys_user:
            self.redirect(users.create_login_url(self.request.uri))
            return

        return func(self, **kwargs)
    return dec


class ApiAuth(webapp2.RequestHandler):
    @admin_only
    @decorator.oauth_aware
    def get(self):
        if not decorator.has_credentials():
            self.redirect(decorator.authorize_url())
            return

        auth = Auth.get_or_insert('auth')
        auth.credentials = decorator.get_credentials()
        auth.put()

        self.redirect('/config')


class ConfigPage(webapp2.RequestHandler):
    @admin_only
    def get(self, error=None, msg=None, flags={}):
        user = User.get(users.get_current_user())
        if not user.superuser:
            self.redirect('/invite',
                          error="Must be an admin to edit configuration.")
            return

        template_values = {
            'config': config,
            'flags': flags,
            'logout': users.create_logout_url('/'),
            'user': user,
            'error': error,
            'msg': msg,
        }
        template = JINJA_ENVIRONMNT.get_template('config.html')
        self.response.write(template.render(template_values))

    @admin_only
    def post(self):
        user = User.get(users.get_current_user())
        if not user.superuser:
            self.redirect('/invite',
                          error="Must be an admin to edit configuration.")
            return

        err = []
        msg = []
        flags = {}
        for name in config.keys():
            v = self.request.get('config_%s' % name)
            if v != "":
                logging.info("Updating %s: %s" % (name, v))
                flags[name] = Config.update(name, v)
                if flags[name] == True:
                    config[name] = v
                elif flags[name] is None:
                    err.append("Failed to update %s." % name)
            else:
                Config.query(Config.name==name).get().key.delete()
                del(config[name])
                msg.append("'%s' deleted." % name)
                logging.info("Deleted key '%s'." % name)

        new_name = self.request.get('new_name')
        if new_name:
            value = self.request.get('new_value')
            logging.info("New config key %s: %s" % (new_name, value))
            Config(name=new_name, value=value).put()
            flags[new_name] = True
            config[new_name] = value


        self.get(error=", ".join(err), msg=", ".join(msg), flags=flags)


class InvitePage(webapp2.RequestHandler):
    @logged_in
    def get(self, template_values={}, msg=None, error=None):
        user = User.get(users.get_current_user())

        if user.superuser:
            invitations = Invitation.query(ancestor=Invitation.dummy())
        else:
            invitations = Invitation.query(Invitation.owner==user.key,
                                           ancestor=Invitation.dummy())
        invitations = invitations\
                      .filter(Invitation.date >= Utils.now())\
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
        user = User.get(users.get_current_user())

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
            logging.info('Invite id: %s, gn: %s', invite.key, gn)
            invite.key.delete()
            msg = 'Invitation withdrawn. '

            if gn:
                gn.key.delete()
                msg += 'Rescheduling gamenight. '
                Gamenight.schedule()

            self.get(msg=msg)
            return


        args = {}
        for k in ['when', 'where', 'priority', 'notes']:
            args[k] = self.request.get(k)

        error = None
        msg = ''
        if args['when']:
            try:
                orig = args['when']
                args['when'] = parser.parse(args['when'].replace('today', ''))
                logging.info('Parsed "%s" as "%s"', orig, args['when'])
            except ValueError:
                error = 'Not sure what you mean by "%s"' % args['when']
                logging.error('Failed to parse when: %s', args['when'])
            else:
                checks = []
                if not time(18, 0, 0) <= args['when'].time() <= time(21, 0, 0):
                    checks.append(args['when'].time().strftime('%I:%M %p'))
                if args['when'].date().weekday() != 5:
                    checks.append(args['when'].date().strftime('%A'))
                if args['when'].date() < Utils.now().date():
                    checks.append(args['when'].date().strftime('%x'))

                if checks:
                    msg += 'Just checking, did you really mean %s? ' % ', '.join(checks)
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

        updated, invite = Invitation.create(args)

        if updated:
            gn = Gamenight.query(Gamenight.invitation==invite.key).get()
            if gn:
                gn.update()
                msg += 'Invitation and gamenight updated! '
            else:
                msg += 'Invitation updated! '

            self.get(msg=msg)
        else:
            msg += 'Invitation created! '
            self.get(msg=msg)


class ProfilePage(webapp2.RequestHandler):
    @logged_in
    def get(self, template_values={}, msg=None, error=None, profile=None):
        user = User.get(users.get_current_user())

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
                template_values['profile'] = User.lookup(profile)

                if not profile:
                    template_values['profile'] = user
                    template_values['error'] = "Couldn't find user %s" % profile


        template = JINJA_ENVIRONMNT.get_template('profile.html')
        self.response.write(template.render(template_values))

    @logged_in
    def post(self):
        user = User.get(users.get_current_user())

        if user.superuser:
            edit = self.request.get('edit', False)
            if edit:
                self.get(profile=edit, msg='Editing %s' % edit)
                return
            profile = User.lookup(self.request.get('pid'))
            profile.superuser = self.request.get('admin')=='on'
        else:
            profile = user

        profile.location = self.request.get('location')
        profile.name = self.request.get('name')
        profile.nag = self.request.get('nag')=='on'
        profile.put()

        self.get(msg='Profile updated!', profile=profile.key.id())


class SchedulePage(webapp2.RequestHandler):
    def get(self):
        days = defaultdict(dict)
        for gn in Gamenight.future(10):
            days[gn.date]['date'] = gn.date
            days[gn.date]['scheduled'] = True
            days[gn.date]['status'] = gn.status
            days[gn.date]['owner'] = gn.owner
            days[gn.date]['time'] = gn.time
            days[gn.date]['where'] = gn.location

        invitations = Invitation.query(Invitation.date >= Utils.now()).\
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
            user = User.get(users.get_current_user())
            template_values.update({
                'logout': users.create_logout_url('/'),
                'user': user,
            })

        template = JINJA_ENVIRONMNT.get_template('schedule.html')
        self.response.write(template.render(template_values))


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
                             date=Utils.saturday(),
                             lastupdate=Utils.now())
            futurenights = []

        updated = gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p')

        invites = Invitation.summary()

        upcoming = {g.date: { 'type': 'gamenight',
                              'date': g.date,
                              'location': g.location }
                            for g in futurenights}
        for week in range(0,5):
            day = (Utils.saturday() + timedelta(week*7)).date()

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

        if gamenight.date and gamenight.time:
            template_values['when'] = datetime.combine(gamenight.date, gamenight.time).strftime('%A, %I:%M %p')

        if gamenight.location:
            template_values['where'] =  gamenight.location

        if gamenight.notes:
            template_values['notes'] =  gamenight.notes

        template = JINJA_ENVIRONMNT.get_template('index.html')
        # Write the submission form and the footer of the page
        self.response.write(template.render(template_values))


# tasks
class NagTask(webapp2.RequestHandler):
    def get(self):
        # don't bother starting to nag before Tuesday 10am
        today = datetime.today()
        email = self.request.get('email', False)
        priority = None
        # sun-tue only check high priority
        if today.weekday() in (6, 0, 1) and not email:
            logging.debug('Only checking high priority invites.')
            priority = 'Insist'

        status = self.request.get('status', None)
        gn = Gamenight.schedule(status=status, priority=priority)
        if gn and gn.status == 'Yes' or not email:
            logging.debug('No need to nag.')
            self.redirect('/')
            return

        # saturday afternoon just give up and say no
        if today.weekday() == 5 and today.hour > 16:
            logging.debug('Giving up on scheduling this week.')
            gn = Gamenight.schedule(status='No', date=today.date())
            self.redirect('/')
            return

        logging.debug('Sending out email template: %s', email)
        subjects = { 'first': 'Want to host gamenight?',
                     'second': 'Still looking to find a host for gamenight this week!' }
        bodies = {'first': "Seems that no one has offered to host gamenight this week. " +
                           "Want to host? Go to http://%(url)s/invite!" ,
                  'second': "We still haven't had anyone volunteer to host gamenight this week. " +
                            "For now, the site will show '%(status)s', but if you'd like to host, " +
                            "please go to http://%(url)s/invite to have people come over.  ",
                 }

        footer = ("\nThanks!\n\n(You asked to get these emails if no one is hosting gamenight. " +
                  "If you want to stop getting these, go to http://%(url)s/profile and uncheck " +
                  "the 'nag emails' option.)")

        message = mail.EmailMessage()
        message.sender = 'Gamenight <%s>' % config.get('sender')
        message.to = message.sender
        message.subject = subjects[email]
        message.body = (bodies[email] + footer) % { 'url': config.get('url', "TBD"), 'status': status }

        message.bcc = [u.key.id() for u in User.query(User.nag==True).fetch()]
        logging.info('Sending nag email to %r', message.to)
        message.send()

        self.redirect('/')

class ResetTask(webapp2.RequestHandler):
    def get(self):
        Gamenight.reset()
        self.redirect('/')

class ScheduleTask(webapp2.RequestHandler):
    def get(self):
        Gamenight.schedule()
        self.redirect('/')


debug = True
application = webapp2.WSGIApplication([
    ('/', MainPage),
    ('/apiauth', ApiAuth),
    ('/config', ConfigPage),
    ('/invite', InvitePage),
    ('/profile', ProfilePage),
    ('/schedule', SchedulePage),
    (decorator.callback_path, decorator.callback_handler()),
], debug=debug)

cron = webapp2.WSGIApplication([
    ('/tasks/nag', NagTask),
    ('/tasks/reset', ResetTask),
    ('/tasks/schedule', ScheduleTask),
], debug=debug)

# vim: set ts=4 sts=4 sw=4 et:
